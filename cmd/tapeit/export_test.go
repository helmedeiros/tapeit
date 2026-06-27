package main

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"This Is The Black Keys": "this-is-the-black-keys",
		"This Is Los Hermanos":   "this-is-los-hermanos",
		"Último Romance":         "último-romance",
		"Harder, Better!":        "harder-better",
		"  spaced  out  ":        "spaced-out",
		"A.K.A. I-D-I-O-T":       "a-k-a-i-d-i-o-t",
		"???":                    "playlist",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
