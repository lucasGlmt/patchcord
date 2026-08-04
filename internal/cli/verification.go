package cli

import (
	"fmt"
	"io"

	"github.com/lucasglmt/patchcord/internal/signing"
	"github.com/lucasglmt/patchcord/internal/trust"
)

// printVerificationStatus warns about a just-installed package's signature
// status on out, matching what internal/trust.CheckPolicy found — silent
// when the package is signed by a key already trusted for id, since that
// is the expected, unremarkable case. requireSignature installs already
// fail before this is ever called (internal/trust.ErrSignatureRequired),
// so this only ever prints warnings, never blocks anything.
func printVerificationStatus(out io.Writer, id string, policy trust.PolicyResult) {
	switch {
	case policy.Outcome.Signed && policy.Trusted:
		return
	case policy.Outcome.Signed:
		fmt.Fprintf(out, "⚠ %s is signed by an untrusted key %s — run `patchcord trust add %s <path-to-pubkey>` to trust it\n",
			id, signing.Fingerprint(policy.Outcome.PublicKey), id)
	default:
		fmt.Fprintf(out, "⚠ %s is not signed\n", id)
	}
}
