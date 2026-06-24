// Package apple talks to the Apple Music API using tokens extracted from the
// music.apple.com web player: a shared developer token (Bearer) plus the user's
// own media-user-token. This avoids a paid Apple Developer membership. See
// docs/DESIGN.md for the trade-offs and how to obtain the tokens.
package apple

import "errors"

// Credentials holds the values extracted from a logged-in music.apple.com
// session, plus the resolved storefront.
type Credentials struct {
	DeveloperToken string `json:"developer_token"`
	UserToken      string `json:"user_token"`
	Storefront     string `json:"storefront"`
}

// Validate checks that the credentials are usable for catalog reads.
func (c Credentials) Validate() error {
	if c.DeveloperToken == "" {
		return errors.New("missing developer token")
	}
	return nil
}

// ValidateForWrite checks that the credentials can write to the library.
func (c Credentials) ValidateForWrite() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.UserToken == "" {
		return errors.New("missing user token (media-user-token) required for library writes")
	}
	return nil
}
