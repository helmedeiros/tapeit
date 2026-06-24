// Package spotify reads a user's own library via the Spotify Web API using the
// Authorization Code flow with PKCE (no client secret), suitable for a CLI.
package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	authorizeURL = "https://accounts.spotify.com/authorize"
	tokenURL     = "https://accounts.spotify.com/api/token"

	// RedirectURI must be registered verbatim in the Spotify app settings.
	RedirectURI  = "http://127.0.0.1:8888/callback"
	redirectPort = "8888"
)

// Scopes required to read the user's own playlists, followed playlists, and
// Liked Songs.
var Scopes = []string{
	"playlist-read-private",
	"playlist-read-collaborative",
	"user-library-read",
}

// Token is a Spotify OAuth token plus the client id needed to refresh it.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
	ClientID     string    `json:"client_id"`
}

func (t Token) expired() bool {
	// Refresh a minute early to avoid races against expiry.
	return time.Now().After(t.Expiry.Add(-time.Minute))
}

// Login runs the interactive PKCE authorization flow and returns a Token.
// It opens the user's browser and serves the loopback redirect to capture the
// authorization code.
func Login(ctx context.Context, clientID string) (Token, error) {
	verifier, challenge, err := pkce()
	if err != nil {
		return Token{}, err
	}
	state, err := randomString(16)
	if err != nil {
		return Token{}, err
	}

	code, err := authorize(ctx, clientID, challenge, state)
	if err != nil {
		return Token{}, err
	}
	return exchangeCode(ctx, clientID, code, verifier)
}

func authorize(ctx context.Context, clientID, challenge, state string) (string, error) {
	q := url.Values{
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {RedirectURI},
		"scope":                 {strings.Join(Scopes, " ")},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
		"state":                 {state},
	}
	authURL := authorizeURL + "?" + q.Encode()

	ln, err := net.Listen("tcp", "127.0.0.1:"+redirectPort)
	if err != nil {
		return "", fmt.Errorf("listen on :%s (is it free?): %w", redirectPort, err)
	}
	defer func() { _ = ln.Close() }()

	resCh := make(chan authResult, 1)
	srv := &http.Server{Handler: callbackHandler(state, resCh), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	fmt.Println("Opening browser to authorize tapeIt with Spotify…")
	fmt.Println("If it doesn't open, paste this URL:\n  " + authURL)
	_ = openBrowser(authURL)

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-resCh:
		return r.code, r.err
	case <-time.After(3 * time.Minute):
		return "", fmt.Errorf("timed out waiting for Spotify authorization")
	}
}

type authResult struct {
	code string
	err  error
}

func callbackHandler(wantState string, resCh chan<- authResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		send := func(code string, err error) {
			select {
			case resCh <- authResult{code, err}:
			default:
			}
		}
		if e := q.Get("error"); e != "" {
			_, _ = fmt.Fprintf(w, "Authorization failed: %s. You can close this tab.", e)
			send("", fmt.Errorf("spotify authorization error: %s", e))
			return
		}
		if q.Get("state") != wantState {
			_, _ = fmt.Fprint(w, "State mismatch. You can close this tab.")
			send("", fmt.Errorf("oauth state mismatch (possible CSRF)"))
			return
		}
		_, _ = fmt.Fprint(w, "tapeIt is authorized. You can close this tab and return to the terminal.")
		send(q.Get("code"), nil)
	}
}

func exchangeCode(ctx context.Context, clientID, code, verifier string) (Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {RedirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}
	return postToken(ctx, clientID, form)
}

func refreshToken(ctx context.Context, t Token) (Token, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {t.RefreshToken},
		"client_id":     {t.ClientID},
	}
	nt, err := postToken(ctx, t.ClientID, form)
	if err != nil {
		return Token{}, err
	}
	// Spotify may omit a new refresh token; keep the old one.
	if nt.RefreshToken == "" {
		nt.RefreshToken = t.RefreshToken
	}
	return nt, nil
}

func postToken(ctx context.Context, clientID string, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Token{}, fmt.Errorf("decode token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || body.AccessToken == "" {
		return Token{}, fmt.Errorf("token endpoint %d: %s %s", resp.StatusCode, body.Error, body.ErrorDesc)
	}
	return Token{
		AccessToken:  body.AccessToken,
		RefreshToken: body.RefreshToken,
		TokenType:    body.TokenType,
		Expiry:       time.Now().Add(time.Duration(body.ExpiresIn) * time.Second),
		ClientID:     clientID,
	}, nil
}

func pkce() (verifier, challenge string, err error) {
	verifier, err = randomString(32) // 43 base64url chars, within the 43-128 range
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomString(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}
