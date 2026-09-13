package warn

import "testing"

func TestHasLink(t *testing.T) {
	cases := map[string]bool{
		"見てね https://example.com/a.zip": true,
		"http://example.com":            true,
		"ftp://example.com/x":           false,
		"ただの本文":                         false,
		"":                              false,
	}
	for in, want := range cases {
		if got := HasLink(in); got != want {
			t.Errorf("HasLink(%q)=%v want %v", in, got, want)
		}
	}
}
