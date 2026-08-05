package embedded

import "testing"

// Files must never error just because the embedding step
// (`make build-embedded-plugins`) hasn't run for this platform yet — a
// freshly checked out repo, or any GOOS/GOARCH the release matrix doesn't
// cross-build for, embeds nothing but the .gitkeep placeholder that keeps
// the embed directive itself satisfied (see platform_*.go and ADR-0059).
// Files must filter that placeholder out either way.
func TestFiles_NeverErrorsAndHidesThePlaceholder(t *testing.T) {
	files, err := Files()
	if err != nil {
		t.Fatalf("Files() error = %v, want nil even with nothing built for this platform", err)
	}

	for _, f := range files {
		if len(f.Name) > 0 && f.Name[0] == '.' {
			t.Fatalf("Files() returned %q, want the .gitkeep placeholder filtered out", f.Name)
		}
		if len(f.Data) == 0 {
			t.Fatalf("Files() returned %q with no data", f.Name)
		}
	}
}
