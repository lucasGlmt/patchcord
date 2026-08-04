# Package Signing & Trust

`.patchcord-plugin`, `.patchcord-app` and `.patchcord-bundle` packages (see [ADR-0042](../../../adr/0042-formats-de-package-plugin-workflow-bundle.md)) can carry integrity and provenance data, added by `pack` and checked by `install` — see [ADR-0043](../../../adr/0043-signature-et-verification-des-packages.md) for the full design rationale. This page covers the mechanism once, since it works identically across [`plugin`](commands/plugin.md), [`app`](commands/app.md) and [`bundle`](commands/bundle.md).

## What `pack` always produces

Every package `pack` command writes a `checksums.json` into the archive — a sha256 digest of every file, keyed by its relative path. This happens whether or not you sign the package: it lets `install` detect a corrupted or tampered archive unconditionally, regardless of any signing policy.

## Signing (optional)

Generate a key pair once:

```bash
patchcord key generate -o my-signing-key
```

This writes `my-signing-key` (private, keep it — never commit it) and `my-signing-key.pub` (public, safe to share). There is no recovery for a lost private key, only generating another one and re-signing.

Pass the private key to any `pack` command to add a `signature.json` (an Ed25519 signature over `checksums.json`):

```bash
patchcord plugin pack ./my-plugin --sign-key my-signing-key
```

Signing a bundle covers its embedded app and workflows too — they are never separately re-verified when the bundle itself installs (`internal/bundles.installEmbeddedApp` calls the same service `app install` on a directory would, not a second package verification).

## Trusting a key (explicit, per id)

A signature alone proves *what* was signed, not that the signer is legitimate for that package id. Approve a public key explicitly:

```bash
patchcord trust add io.patchcord.example-text ./my-signing-key.pub
```

Trust is bound to the **pair** (id, public key) — approving a key for one id never trusts it for another, even from the same signer. `patchcord trust list [id]` and `patchcord trust remove <id> <pubkey-path>` manage the store.

## What `install` does with all this

| Package state | Default `install` | `install --require-signature` |
|---|---|---|
| No `checksums.json` (old-style, or `pack` without signing support) | Installs, no warning | Installs (there is nothing to verify — see below) |
| `checksums.json` present, matches | Installs | Installs only if also signed and trusted |
| `checksums.json` present, **does not match** | **Fails** (`ErrChecksumMismatch`) | **Fails** |
| Signed, key not yet trusted for this id | Installs, prints a warning | **Fails** |
| Signed, key trusted for this id | Installs, silent | Installs, silent |
| `signature.json` present but cryptographically invalid | **Fails** (`ErrInvalidSignature`) | **Fails** |

A checksum mismatch or an invalid signature is never a warning, regardless of `--require-signature` — that distinguishes "nobody signed this" (a policy question) from "this archive doesn't match what it claims to contain" (not a policy question at all).

`--require-signature` only makes sense against an actual package: passing it while installing a raw plugin executable or an app directory (no archive, nothing to verify) is a command error, not a silent no-op.
