package domain

import "testing"

func TestBaseAlbumName_FoldsEditionsOntoOneRecord(t *testing.T) {
	// Coverage is measured against an album's track count, so the same record
	// reached under two edition names must reduce to one identity or the
	// denominator silently changes.
	cases := map[string]string{
		"Future Nostalgia (Deluxe)":                "future nostalgia",
		"Everyday Robots (Special Edition)":        "everyday robots",
		"FOUR (The Ultimate Edition)":              "four",
		"Brothers (Deluxe Remastered Anniversary)": "brothers",
		"Song 2 - 2012 Remaster":                   "song 2",
		"Take Me Home (Yearbook Edition)":          "take me home",
		"El Camino":                                "el camino",
	}
	for in, want := range cases {
		if got := BaseAlbumName(in); got != want {
			t.Errorf("BaseAlbumName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBaseAlbumName_KeepsMeaningfulParentheses(t *testing.T) {
	// Not every parenthetical marks an edition. Stripping them all would merge
	// genuinely different records, which is a worse error than missing a fold.
	cases := map[string]string{
		"This Must Be the Place (Naive Melody)": "this must be the place naive melody",
		"(What's the Story) Morning Glory?":     "whats the story morning glory",
		"Sgt. Pepper's Lonely Hearts Club Band": "sgt peppers lonely hearts club band",
	}
	for in, want := range cases {
		if got := BaseAlbumName(in); got != want {
			t.Errorf("BaseAlbumName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrimaryArtist_ReducesToTheLeadAct(t *testing.T) {
	cases := map[string]string{
		"Gorillaz, George Benson":          "gorillaz",
		"Gorillaz & George Benson":         "gorillaz",
		"Post Malone feat. Morgan Wallen":  "postmalone",
		"Shawn Mendes with Camila Cabello": "shawnmendes",
		"The Black Keys":                   "theblackkeys",
		"":                                 "",
	}
	for in, want := range cases {
		if got := PrimaryArtist(in); got != want {
			t.Errorf("PrimaryArtist(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAlbumKey_IsStableAcrossServices(t *testing.T) {
	// The same record as Apple spells it and as Spotify spells it. If these
	// disagree the library-corroboration signal silently reads as zero.
	apple := AlbumKey("Future Nostalgia (Deluxe)", "Dua Lipa")
	spotify := AlbumKey("Future Nostalgia", "Dua Lipa, DaBaby")
	if apple != spotify {
		t.Errorf("cross-service key mismatch: %q vs %q", apple, spotify)
	}
	if AlbumKey("El Camino", "The Black Keys") == AlbumKey("Brothers", "The Black Keys") {
		t.Error("different records must not collide")
	}
	if AlbumKey("Greatest Hits", "Queen") == AlbumKey("Greatest Hits", "ABBA") {
		t.Error("a shared album title must not collide across artists")
	}
}

func TestAlbumKey_IsCaseAndPunctuationInsensitive(t *testing.T) {
	if AlbumKey("IS THIS IT", "the strokes") != AlbumKey("Is This It", "The Strokes") {
		t.Error("case must not change identity")
	}
}

func TestAlbumKey_KeepsSymbolOnlyTitlesDistinct(t *testing.T) {
	// Titles made entirely of symbols — Ed Sheeran's ÷ and ×, Blur's 13 — reduce
	// to nothing once punctuation is stripped, so every such record by one artist
	// would collide into a single album and silently pool their evidence.
	if got := BaseAlbumName("÷"); got == "" {
		t.Error("a symbol-only title must still yield a name")
	}
	if AlbumKey("÷", "Ed Sheeran") == AlbumKey("×", "Ed Sheeran") {
		t.Error("÷ and × are different records by the same artist")
	}
	if AlbumKey("÷ (Deluxe)", "Ed Sheeran") != AlbumKey("÷", "Ed Sheeran") {
		t.Error("editions of a symbol-titled record should still fold together")
	}
}
