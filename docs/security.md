# Security

**Audience:** operators and reviewers evaluating how secrets are protected.
**Covers:** what exists today. To report a vulnerability, see
[SECURITY.md](../SECURITY.md).

All deployment secrets are supplied through the environment; nothing sensitive
is committed to the repository or baked into the container image.

## Encryption at rest

- **Envelope encryption.** Every stored secret has its own random data key (DEK)
  that encrypts the value with AES-256-GCM; the DEK is then wrapped by a
  key-encryption key (KEK). Rotating the KEK re-wraps data keys rather than
  re-encrypting every secret.
- **Pluggable KEK provider.** With `KEK_PROVIDER=local` the KEK is derived from
  `RESOURCE_SECRET_KEY`. With `azure_key_vault` the KEK is an RSA key that lives
  inside Key Vault and never leaves it — wrap and unwrap happen in the vault,
  authenticated by the deployment's ambient Azure identity. In that mode a
  database dump plus every copy of `RESOURCE_SECRET_KEY` still decrypts nothing
  without live, auditable Key Vault access. Existing rows re-wrap automatically
  at startup when the provider changes, and back again if reverted.
- **Secret classes.** Shared/team secrets and app-scope operational credentials
  wrap under the org KEK. Personal secrets are sealed to the owner's per-user
  vault public key and are unreadable by the server, admins, or a database
  operator — only the owner's unlocked session can decrypt them.
- **What else is protected.** Sensitive Administration settings (Entra client
  secret, SMTP password, RDP signing material) are encrypted at rest with the
  same scheme. Session and browser-extension tokens are stored only as SHA-256
  hashes, so a database dump exposes no usable bearer tokens.
- Use a unique `RESOURCE_SECRET_KEY` per deployment — the backend refuses to
  start without one, or with the legacy shared development key.

## Personal vaults

Any user can save **personal** secrets that are private to them — not even an
administrator or a database operator can read them. Each user gets a vault
keypair; personal secrets are sealed to the vault's public key, and only the
owner can unlock the private key.

- **Unlocking.** Local-account users' vaults unlock automatically from their
  login password. Microsoft (SSO) users are prompted once per session, right
  after sign-in, to unlock with a **passkey** — Windows Hello, Touch ID, or a
  security key (WebAuthn PRF) — or a **passphrase** fallback for devices without
  a platform authenticator.
- **Unlock methods are peers.** Any one of them opens the vault; none of them
  *is* the key. Users add, rename, and remove their own methods from vault
  settings; the last remaining method and the login-password wrap are protected
  from removal.
- **Saving never prompts.** Sealing to a public key needs no secret, so saving a
  personal password works from any session — including the extension's silent
  save on a machine with no enrolled passkey. Reading is what requires an
  unlocked session.
- **Account lifecycle.** Admins create users with an **invite link** (the user
  sets their own password, so no admin ever knows it) or with a temporary
  password. Users change their own password without losing personal secrets. An
  admin-forced **reset destroys** the user's vault and personal secrets by
  design — a reset cannot recover secrets only the user's credential could open.
- **Recovery.** A personal vault is intentionally unrecoverable without the
  owner's credential: losing the sole passkey with no passphrase means the
  personal secrets are gone. Operationally this is treated as a security event,
  not something the app recovers. The mitigation is having more than one unlock
  method, not recovery codes.
- **Personal ↔ shared.** A saved password can be switched between personal and
  shared. Making a shared password personal is owner-only; making a personal
  password shared requires the owner's unlocked session. Both are a re-wrap of
  the data key, never a plaintext round-trip.

## Sessions and perimeter

- Web sessions ride an httpOnly cookie — no token in localStorage or in redirect
  URLs — with a CSRF origin check on state-changing requests. The browser
  extension holds a separate bearer-token session.
- Sessions are listable and revocable. A user sees their own web and extension
  sessions (client label, IP, last activity) and can end any one or all others;
  an admin can do the same for any user without blocking them. Revocation is
  immediate — every request is checked against the database — and a revoked
  session loses its copy of the unlocked vault key, so a leaked token cannot
  read personal secrets afterwards. Changing your password ends all your other
  sessions. The client IP and user agent are recorded once when a session is
  opened and deleted with it; expired rows are purged after seven days. Web
  sessions last 24 hours and extension sessions 30 days by default; both are
  configurable (see [configuration](configuration.md)).
- Login and vault-unlock endpoints have account lockout and per-IP rate limiting.
- The frontend ships CSP, HSTS, and related security headers; the API sets
  equivalent headers on its responses.
- Authentication, vault, unlock-method changes, and session revocations are
  audited alongside resource events.
- The Generator page produces passwords, keys and keypairs in the browser with
  the Web Crypto API; nothing generated is transmitted or stored. Self-signed
  certificates are produced on the server because the browser cannot package
  PKCS#12, returned once, and never persisted or logged. Access is per part
  through the rights `generator.passwords`, `generator.keysandtokens`,
  `generator.keypairs` and `generator.certificates`, assigned directly or
  through local groups like every other right; a user with none of them does
  not see the page. Only generation *settings* (length, character classes,
  key size, certificate defaults) are remembered per user — never generated
  values, names or passwords.

## Storage and secret modes

- Resource metadata and secret material are stored separately, in `resources`
  and `resource_secrets`.
- Secret modes: `inline` (app-managed, always envelope-encrypted),
  `external_reference` (pointer to a value owned elsewhere), `azure_key_vault`
  (fetched live, never persisted locally), `prompt_on_launch` (asked for at
  launch time), and `none` (passwordless portal logins).
- Key Vault-backed resources keep metadata locally but fetch the live secret
  value from Azure on demand.
- App registration sync stores credential expiry metadata and owner snapshots,
  but never stores client secret values.
- Empty `allowed_groups` and `allowed_users` means the resource is visible to
  everyone who can access that category.
- Launch actions return structured payloads rather than executing anything
  server-side; the launcher redeems a one-time ticket so secrets never travel in
  a browser URI.
- Audit events are stored in PostgreSQL and exposed through admin and per-user
  views.

## Permission model

Viewing, revealing, copying, launching, and filling are distinct capabilities,
evaluated per category from the user's resolved rights and then per object from
its sharing lists. Reveal and copy are unified into one "may the people this is
shared with use the secret" decision — anyone who can put a secret on the
clipboard can read it, so splitting them bought nothing.

Connections can opt into a readable secret because some targets refuse handed-over
credentials outright (`fPromptForPassword`), and a connection the user cannot
enter is worse than one whose password they can read.
