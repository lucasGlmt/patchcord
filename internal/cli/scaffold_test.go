package cli

import "testing"

func TestScaffoldDirName(t *testing.T) {
	cases := map[string]string{
		"io.patchcord.example-text": "example-text",
		"example-text":              "example-text",
		"io.patchcord.":             "io.patchcord.",
	}
	for id, want := range cases {
		if got := scaffoldDirName(id); got != want {
			t.Fatalf("scaffoldDirName(%q) = %q, want %q", id, got, want)
		}
	}
}
