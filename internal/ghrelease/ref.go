// Package ghrelease resolves and downloads a pre-built package asset
// attached to a GitHub repository's Releases — a second, parallel install
// source alongside internal/registry's configured-registry lookups
// (ADR-0067). It never clones or builds a repository: it only calls
// GitHub's public REST API to find one release asset and downloads it.
// Install-time CLI tooling only; not part of any versioned public
// boundary (api/, sdk/, plugin protocol).
package ghrelease

import "regexp"

// Ref is a parsed reference to a GitHub-hosted release asset:
// github.com/<owner>/<repo>[@<tag>]. Tag == "" means "resolve to the
// repository's latest release"; a non-empty Tag is used verbatim as a
// GitHub release tag name (no "v" prefix guessing).
type Ref struct {
	Owner string
	Repo  string
	Tag   string
}

// refPattern matches only the github.com host, deliberately: a future
// host (gitlab.com/..., etc.) is added as its own sibling parser/package
// later, not by loosening this pattern — ParseRef's signature and Ref's
// shape do not need to change for that, since callers already treat "not
// recognized" (ok == false) as "try the next source", never as an error
// in itself.
var refPattern = regexp.MustCompile(`^github\.com/([^/@\s]+)/([^/@\s]+)(?:@(.+))?$`)

// ParseRef reports whether arg matches the github.com/<owner>/<repo>[@<tag>]
// syntax and, if so, the parsed Ref. It never returns an error: an
// unrecognized arg simply reports ok == false so a caller can fall back to
// another install source (a local file, a configured registry id, ...).
func ParseRef(arg string) (ref Ref, ok bool) {
	m := refPattern.FindStringSubmatch(arg)
	if m == nil {
		return Ref{}, false
	}
	return Ref{Owner: m[1], Repo: m[2], Tag: m[3]}, true
}
