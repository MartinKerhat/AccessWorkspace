# API

**Audience:** contributors and integrators. **Covers:** the HTTP surface that exists today.

All endpoints are under `/api`. Browser clients authenticate with an httpOnly
session cookie and are subject to a CSRF origin check on state-changing requests;
the browser extension uses its own bearer-token session issued through a one-time
exchange token.

## Auth

- `GET /api/auth/bootstrap`
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/auth/me`
- `GET /api/auth/microsoft/start`
- `GET /api/auth/microsoft/callback`
- `POST /api/auth/password` — self-service password change
- `POST /api/auth/invite/accept` — set password from an invite link

## Personal vault

- `GET /api/auth/vault` — status: has-vault, unlocked, methods, passkeys
- `POST /api/auth/vault/setup` / `POST /api/auth/vault/unlock` — passphrase
- `POST /api/auth/vault/passphrase` — add a passphrase to an unlocked vault
- `POST /api/auth/vault/passkey/setup` / `.../unlock` / `.../add` — WebAuthn PRF
- `POST /api/auth/vault/lock`

Reveal, fill, and launch of a personal secret return `423 Locked` when the vault
is not unlocked in the current session, so the UI can prompt.

## Resources

- `GET /api/resources`
- `POST /api/resources`
- `GET /api/resources/{id}`
- `PUT /api/resources/{id}`
- `PUT /api/resources/{id}/app-registration-notifications`
- `POST /api/resources/{id}/archive`
- `POST /api/resources/{id}/reveal`
- `POST /api/resources/{id}/launch`

## Key Vault

- `GET /api/keyvault/discover`
- `POST /api/keyvault/import`
- `POST /api/keyvault/sync`

## App registrations

- `GET /api/appregistrations/discover`
- `POST /api/appregistrations/import`
- `POST /api/appregistrations/sync`

## Admin

- `GET /api/admin/config`
- `PUT /api/admin/config`
- `GET /api/admin/local-groups`
- `POST /api/admin/local-groups`
- `PUT /api/admin/local-groups/{name}`
- `GET /api/admin/users`
- `POST /api/admin/users` — create; optionally issues an invite link
- `GET /api/admin/users/{id}` / `PUT /api/admin/users/{id}` / `DELETE /api/admin/users/{id}`
- `POST /api/admin/users/{id}/invite` — reissue invite link
- `POST /api/admin/users/{id}/reset-password` — destroys the vault; issues a reset link
- `GET /api/admin/notification-deliveries`
- `GET /api/admin/archived-resources`
- `POST /api/admin/archived-resources/{id}/restore`

## Activity and audit

- `GET /api/audit`
- `GET /api/me/activity`
- `GET /api/me/notifications`
- `POST /api/me/notifications/{id}/read`

## Launcher

The launcher is not a general API client. The web app requests a one-time launch
ticket, and the launcher redeems it against the deployment that issued it — so
secrets never appear in a browser URI. The launcher's own loopback bridge
(`127.0.0.1:47654`) exposes `/status` (version, platform, per-feature
capabilities) and `/launch`.
