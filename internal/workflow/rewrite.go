package workflow

import (
	"fmt"
	"regexp"
)

// topLevelVersion matches a workflow definition's top-level `version:`
// field (always an unindented, unquoted integer scalar — see Scaffold's
// own template) up to and including its digits, leaving anything after
// them (an inline comment, trailing whitespace) untouched.
var topLevelVersion = regexp.MustCompile(`(?m)^version:\s*\d+`)

// RewriteVersion returns source with its top-level `version:` field
// replaced by version — byte-for-byte unchanged otherwise, including
// comments and formatting anywhere else in the file. It is a no-op
// (returns source unchanged, not even reallocated) when source's declared
// version already equals version.
//
// This exists for runs.InstallWorkflowAtVersion (ADR-0055): a dev-mode
// auto-assigned version can differ from what the file itself declares
// (the file is never rewritten on disk — only InstallDir's dev-only
// install path picks a different version than the one written down). The
// *stored* copy must still declare the version it is actually recorded
// under, or re-parsing it (LatestWorkflow, WorkflowSource, `workflow
// export`) would report the stale, on-disk version instead of the one the
// row is keyed on.
func RewriteVersion(source []byte, version int) []byte {
	replacement := fmt.Sprintf("version: %d", version)
	if topLevelVersion.Match(source) && string(topLevelVersion.Find(source)) == replacement {
		return source
	}
	return topLevelVersion.ReplaceAll(source, []byte(replacement))
}
