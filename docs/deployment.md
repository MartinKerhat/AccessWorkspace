# Deployment

**Audience:** operators running this in production. **Covers:** what exists today.

The container images are environment-agnostic and are the same artifacts you run
locally. Configuration comes entirely from the environment — see
[Configuration](configuration.md).

## Typical flow

1. Build and push the `backend` and `frontend` images to your registry (e.g. ACR).
   Your local `docker compose up` builds these same images; production does not use
   the compose file itself.
2. Deploy the images with your platform (e.g. Kubernetes), injecting configuration:
   - non-secret values (`APP_ENV=production`, `AUTH_MODE`, URLs, flags) via a ConfigMap
   - secret values (`RESOURCE_SECRET_KEY`, `DATABASE_URL` or DB password,
     `ENTRA_CLIENT_SECRET`, bootstrap admin password) via a Secret
3. Point `DATABASE_URL` at your managed/external Postgres with `sslmode=require`.
   Do not run the demo Postgres container in production.
4. Set `AUTO_MIGRATE=true` so schema migrations run on startup, and leave
   `SEED_ON_START` / `RESET_DB_ON_START` at `false` (with `APP_ENV=production` these
   are enforced).
5. Provide `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD` so the first admin
   is created automatically on the initial deploy — no manual database access needed.

## Hardening (optional but recommended on Azure)

- **KEK in Key Vault.** Set `KEK_PROVIDER=azure_key_vault` with `KEK_VAULT_URL` /
  `KEK_KEY_NAME` so the key-encryption key lives in Key Vault instead of being
  derived from `RESOURCE_SECRET_KEY`. Grant the backend's identity `wrapKey` /
  `unwrapKey` ("Key Vault Crypto User") on that key. A dump plus the app's env
  then decrypts nothing without live, auditable Key Vault access.
- **Workload identity for reads.** On AKS, give the backend a workload identity
  (federated credential) that holds the Graph and Key Vault read permissions,
  and enable the "reader uses ambient identity" toggle in Administration. The
  Entra client secret then serves only Microsoft sign-in, and no vault-capable
  secret is stored in the database. The KEK unwrap uses the same identity.
- Both changes are re-wrap-safe and reversible: switching the KEK provider
  re-wraps stored data keys at startup, and the reader toggle can be turned off
  to fall back to the stored client secret.

## Serving launcher and extension downloads

The workspace offers the desktop launcher and browser extension as downloads.
`ARTIFACTS_SOURCE` decides where that file list comes from: a mounted directory
(`local`), an Azure Blob container (`blob`), or this repository's GitHub releases
(`github`, which keeps a deployment current without any per-deployment upload
step). See [Browser Extension Distribution](browser-extension-distribution.md)
for how releases are produced.

## Behind a reverse proxy or ingress

Anything handed to an **external agent** must be an absolute URL: the browser
extension's workspace address and the launcher's ticket-resolve URL are derived
from the request origin rather than a relative API base. Same-origin page fetches
stay relative and need nothing.

Two settings must be the real public `https://` origin in production, or
sign-in and emailed links break in ways that never appear locally:

- `FRONTEND_URL` — post-login redirect target, CORS, and the base for invite and
  password-reset links
- the Entra redirect URI (Administration, or `ENTRA_REDIRECT_URI`) — sent
  verbatim, with no scheme rewriting

## Runtime shape

- one API service
- one frontend artifact
- one PostgreSQL dependency
- the backend runs its Key Vault and app registration sync loops in-process
- no separate worker deployment
