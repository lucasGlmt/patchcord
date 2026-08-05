package version

import "testing"

func TestString(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		date    string
		want    string
	}{
		{
			name:    "defaults",
			version: "dev",
			commit:  "none",
			date:    "unknown",
			want:    "dev (commit none, built unknown)",
		},
		{
			name:    "tagged release",
			version: "0.1.0",
			commit:  "abc1234",
			date:    "2026-08-05T12:00:00Z",
			want:    "0.1.0 (commit abc1234, built 2026-08-05T12:00:00Z)",
		},
		{
			name:    "untagged build ahead of last release",
			version: "0.1.0-3-gabcdef",
			commit:  "abcdef1",
			date:    "2026-08-05T12:00:00Z",
			want:    "0.1.0-3-gabcdef (commit abcdef1, built 2026-08-05T12:00:00Z)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origVersion, origCommit, origDate := Version, Commit, Date
			t.Cleanup(func() { Version, Commit, Date = origVersion, origCommit, origDate })

			Version, Commit, Date = tt.version, tt.commit, tt.date

			if got := String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
