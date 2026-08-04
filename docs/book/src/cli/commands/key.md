# patchcord key

Manage package signing keys. See [Package Signing & Trust](../package-signing.md) for the full mechanism. Unlike `trust` and `secret`, `key` never touches the agent's database — a signing key is a packaging-time developer tool.

## `generate`

```bash
patchcord key generate -o my-signing-key
```

Writes a new Ed25519 key pair: `-o` (private key) and `-o.pub` (public key, e.g. `my-signing-key.pub`). Defaults to `patchcord-signing-key` in the current directory if `-o/--output` is omitted. Prints the public key's fingerprint for cross-checking.

The private key is passed to `plugin pack --sign-key`/`app pack --sign-key`/`bundle pack --sign-key`. Distribute the public key to whoever should `patchcord trust add` it. There is no recovery for a lost private key, only generating another one and re-signing.
