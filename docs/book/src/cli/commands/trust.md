# patchcord trust

Manage the trust store for package signing keys. See [Package Signing & Trust](../package-signing.md) for the full mechanism. Trust is bound to the pair (package id, public key) — approving a key for one id never trusts it for another.

## `add <id> <pubkey-path>`

```bash
patchcord trust add io.patchcord.example-text ./my-signing-key.pub
```

Approves the public key at `pubkey-path` (as written by `patchcord key generate`) to sign packages for `id`. Re-adding the same pair updates its label instead of failing.

## `list [id]`

```bash
patchcord trust list
patchcord trust list io.patchcord.example-text
```

Lists every trusted key, or only those trusted for `id` when given: `<id>  <fingerprint>  <label>  <trusted-at>`.

## `remove <id> <pubkey-path>`

```bash
patchcord trust remove io.patchcord.example-text ./my-signing-key.pub
```

Revokes trust for that (id, key) pair. Fails if the pair was never trusted.
