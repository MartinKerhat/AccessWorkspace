# Access Workspace

Monorepo for an internal operational access workspace. It lets company users sign in through local development mode or Microsoft sign-in, browse category-based resources their groups can use, reveal approved secrets on demand, request launch payloads for SSH/RDP/web items, keep private per-user passwords in a personal vault, and inspect activity and audit history. Admins can manage local groups and users, configure Azure/Entra and Key Vault sources, and sync/import Key Vault metadata without duplicating live secret values locally.

Secrets are protected with envelope encryption: every stored secret has its own data key, wrapped by a key-encryption key that can live in Azure Key Vault (never leaving it), so neither a database dump nor a leaked application key is enough to decrypt them. Personal passwords go further — they are sealed to a per-user vault that not even an administrator or database operator can read. See [Data and security notes](#data-and-security-notes).

## Repo layout

- `backend`: Go API, PostgreSQL migrations, seed command, and tests
- `frontend`: React + TypeScript UI
- `deploy`: Dockerfiles and deployment-oriented assets
- `docs`: product, architecture, and roadmap docs

## Documentation

- [Product Vision](docs/product-vision.md)
- [Architecture](docs/architecture.md)
- [App Config Module Spec](docs/app-config-module-spec.md)
- [Domain Model](docs/domain-model.md)
- [Object Model Spec](docs/object-model-spec.md)
- [Roadmap](docs/roadmap.md)
- [Browser Extension Distribution](docs/browser-extension-distribution.md)

## What's included

- Auth entry flow with local development sign-in and Microsoft sign-in bootstrap
- Category-based workspace views for Connections, Key Vault, App registrations, Passwords, Activity, Audit, and Administration
- Group-based resource visibility with app-local group management
- Search and filtering by resource metadata
- Resource detail views with reveal and launch actions where allowed
- Admin resource create, update, archive, and restore flows
- Azure Key Vault discovery, batch import, manual sync, automatic background sync, and optional auto-import defaults
- Azure app registration discovery, batch import, manual sync, automatic background sync, owner snapshots, and secret/certificate expiry metadata
- App registration expiry notification policies with global defaults plus per-app and per-credential overrides
- In-app notification center for app registration credential reminders
- Administration user directory with effective-access inspection, direct group assignment, workspace blocking, local user creation, invite links, and admin-forced password reset
- Personal passwords: a per-user encrypted vault, unlocked by a passkey (Windows Hello / Touch ID / security key) or a passphrase, that administrators and database operators cannot read
- Self-service password change that preserves personal secrets, and switching a saved password between personal and shared
- Envelope encryption for all stored secrets with a pluggable key-encryption-key provider (a local key or Azure Key Vault)
- Connections foundation for RDP and SSH with grouped folder navigation, launcher-ready metadata, browser-triggered launcher handoff attempts, and encrypted stored credentials
- Archived Key Vault view for admins inside the Key Vault workspace area
- Audit logging for view, reveal, launch, create, update, archive, and restore actions
- My recent activity page
- PostgreSQL schema migrations and demo seed data
- Docker Compose local environment

## Authentication modes

The app supports two sign-in paths, selected with the `AUTH_MODE` variable:

- `dev` — the default; reports a development label and a Microsoft-login hint to the UI
- `entra` — Microsoft sign-in backed by runtime Azure/Entra configuration from Administration

Local username/password login is available in **both** modes; `AUTH_MODE` only changes the label and hint reported to the frontend. Every deployment therefore has a working local sign-in for its administrator.

### Getting an initial login

There are two ways to get the first user into a fresh database:

- **Bootstrap admin (recommended for real deployments):** set `BOOTSTRAP_ADMIN_USERNAME` and `BOOTSTRAP_ADMIN_PASSWORD`. On the first startup against an empty user table the app creates that admin, who can then sign in and configure everything else. It is idempotent — once any user exists it never runs again. See [Configuration](#configuration).
- **Demo seed (local/dev only):** when `SEED_ON_START=true`, the backend seeds demo users and sample resources. This is for local exploration and is blocked when `APP_ENV=production`.

Seeded demo users (only present when `SEED_ON_START=true`; password `123456`):

- `alice`: admin, groups `ops-admins`, `platform`, `engineering`
- `sam`: groups `support`, `engineering`
- `nina`: groups `network`, `platform`
- `wendy`: groups `web`, `support`

## Personal passwords (per-user vault)

Any user can save **personal** passwords that are private to them — not even an
administrator or a database operator can read them. Each user gets a personal
vault (an asymmetric keypair); personal secrets are sealed to the vault's public
key, and only the owner can unlock the private key.

- **Unlocking.** Local-account users' vaults are unlocked automatically from
  their login password. Microsoft (SSO) users are prompted once per session,
  right after sign-in, to unlock with a **passkey** — Windows Hello, Touch ID,
  or a security key (WebAuthn PRF) — or a **passphrase** fallback for devices
  without a platform authenticator. The passkey unlock has nothing to remember.
- **Saving vs reading.** Saving a personal password never needs an unlock (it
  seals to the public key); revealing, filling, or launching one requires the
  vault unlocked in the current session, and returns `423` otherwise so the UI
  can prompt.
- **Account lifecycle.** Admins create users with an **invite link** (the user
  sets their own password, so no admin ever knows it) or with a temporary
  password. Users change their own password without losing personal secrets. An
  admin-forced **reset destroys** the user's vault and personal secrets by
  design — a reset cannot recover secrets only the user's credential could open.
- **Recovery.** A personal vault is intentionally unrecoverable without the
  owner's credential: losing the sole passkey with no passphrase means the
  personal secrets are gone. Operationally this is treated as a security event
  (the account is removed), not something the app recovers.
- **Personal ↔ shared.** A saved password can be switched between personal and
  shared. Making a shared password personal is owner-only (it seals the secret
  to the owner's vault); making a personal password shared requires the owner's
  unlocked session and re-encrypts it under the org key.

## Local run with Docker

1. From the repo root, copy the environment template and fill in values:

   ```powershell
   Copy-Item .env.example .env
   ```

   At minimum set `RESOURCE_SECRET_KEY` and `POSTGRES_PASSWORD`. Compose reads `.env`
   and fails fast with a clear message if a required secret is missing. `.env` is
   gitignored and must never be committed.

2. Run `docker compose up --build`.
3. Open [http://localhost:4173](http://localhost:4173).
4. The backend API is available at [http://localhost:8080](http://localhost:8080).

The backend runs migrations automatically. For local convenience the compose file
defaults `SEED_ON_START=true`, so demo users and sample data are available out of the
box. Set `SEED_ON_START=false` (or `RESET_DB_ON_START=true` for a clean reset) in `.env`
to change that.

## Local run without Docker

### Backend

1. Start PostgreSQL locally and create a database named `access_workspace`.
2. From [`backend`](backend), run:

```powershell
$env:DATABASE_URL="postgres://postgres:postgres@localhost:5432/access_workspace?sslmode=disable"
$env:RESOURCE_SECRET_KEY="a-unique-local-dev-key"
$env:AUTO_MIGRATE="true"
$env:RESET_DB_ON_START="true"
$env:SEED_ON_START="true"
go run ./cmd/server
```

`RESOURCE_SECRET_KEY` is required — the server refuses to start without it. Leave
`RESET_DB_ON_START` unset or set it to `false` if you want to preserve local database
state. See [Configuration](#configuration) for the full list of variables.

To seed manually instead of on startup:

```powershell
go run ./cmd/seed
```

### Frontend

From [`frontend`](frontend), run:

```powershell
npm install
npm run dev
```

Then open [http://localhost:5173](http://localhost:5173).

## Configuration

All configuration is read from environment variables. The application has no
baked-in secrets: secret values must be provided by the environment, and the
backend fails fast at startup if a required one is missing.

- **Local development:** copy `.env.example` to `.env` and fill in values. Docker
  Compose and the guidance above read from it. `.env` is gitignored; only
  `.env.example` (a template with no values) is committed.
- **Production / Kubernetes:** the same variables are injected into the container
  by the platform — non-secret values via a ConfigMap, secret values via a Secret.
  The container image itself is environment-agnostic and contains no configuration.

### Variables

| Variable | Required | Default | Notes |
| --- | --- | --- | --- |
| `RESOURCE_SECRET_KEY` | **yes** | — | Master key. With `KEK_PROVIDER=local` it wraps every per-secret data key; with `azure_key_vault` it is still needed to read secrets written before the switch. Generate a unique value per deployment (`openssl rand -base64 32`). Startup fails if empty or set to the legacy shared dev key. |
| `KEK_PROVIDER` | no | `local` | Where per-secret data keys are wrapped: `local` (derived from `RESOURCE_SECRET_KEY`; works anywhere) or `azure_key_vault` (an RSA key inside Key Vault that never leaves it). Existing rows are re-wrapped automatically at startup when this changes. |
| `KEK_VAULT_URL` | when `azure_key_vault` | — | Key Vault URL holding the KEK key, e.g. `https://myvault.vault.azure.net`. |
| `KEK_KEY_NAME` | when `azure_key_vault` | `access-workspace-kek` | Name of the RSA key in that vault. The backend authenticates with its ambient Azure identity (workload identity on AKS, managed identity, or env credentials) and needs `wrapKey`/`unwrapKey` on the key. |
| `DATABASE_URL` | yes | local dev value | Postgres connection string. In production point it at your managed/external database and use `sslmode=require`. |
| `POSTGRES_PASSWORD` | yes (compose) | — | Used by the compose Postgres container and to build `DATABASE_URL` for local dev only. |
| `APP_ENV` | no | empty | Only the value `production` (case-insensitive) enables runtime protection: the app refuses to start if `SEED_ON_START` or `RESET_DB_ON_START` is true. Any other value runs unrestricted. |
| `AUTH_MODE` | no | `dev` | `dev` or `entra` (see [Authentication modes](#authentication-modes)). |
| `AUTO_MIGRATE` | no | `true` | Run database migrations on startup. Safe in production. |
| `SEED_ON_START` | no | `false` | Seed demo data. Dev only — blocked when `APP_ENV=production`. |
| `RESET_DB_ON_START` | no | `false` | Drop and recreate the schema. Dev only — blocked when `APP_ENV=production`. |
| `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD` | no | — | Create the first admin on an empty database. Must be set together; password ≥ 8 chars. Idempotent. |
| `BOOTSTRAP_ADMIN_DISPLAY_NAME` / `BOOTSTRAP_ADMIN_EMAIL` | no | — | Optional metadata for the bootstrapped admin. |
| `HTTP_ADDR` | no | `:8080` | Backend listen address. |
| `FRONTEND_URL` | no | dev value | Frontend URL for CORS/redirects, and the base for emailed invite / password-reset links — set it to the real public URL in production. |
| `ENTRA_TENANT_ID` / `ENTRA_CLIENT_ID` / `ENTRA_CLIENT_SECRET` | when `AUTH_MODE=entra` | — | Microsoft Entra app credentials. Startup fails if `AUTH_MODE=entra` and any are missing. |
| `ENTRA_AUTHORITY` / `ENTRA_REDIRECT_URI` / `ENTRA_GROUP_SOURCE` / `ENTRA_DIRECT_RIGHTS_JSON` | no | see `.env.example` | Additional Entra settings. |
| `ARTIFACTS_SOURCE` | no | `local` | Where downloadable builds (launcher, extensions) are listed from: `local` (a directory) or `blob` (Azure Blob container). See [`artifacts/README.md`](artifacts/README.md). |
| `ARTIFACTS_DIR` | when `local` | `/data/downloads` | Filesystem root of the artifact folders. Dev bind-mounts `./artifacts`; prod mounts a volume. |
| `ARTIFACTS_BLOB_CONTAINER_URL` / `ARTIFACTS_BLOB_SAS` | when `blob` | — | Azure Blob container URL and optional SAS token (list + read). |
| `CHROME_WEB_STORE_URL` / `FIREFOX_EXTENSION_URL` | no | — | Browser-extension store listings. When set, the store becomes the primary install action and direct download the fallback. |
| `VITE_API_BASE_URL` | no (build) | dev value | Frontend build-time API base URL. |

Key Vault browsing and app-registration reads authenticate either with the
Entra client secret stored in Administration, or — when the **"reader uses
ambient identity"** toggle is enabled there — with the deployment's ambient
Azure identity (workload identity / managed identity), so no vault-capable
secret needs to live in the database. That toggle is a runtime admin setting,
not an environment variable.

## Deployment (production)

The container images are environment-agnostic and are the same artifacts you run
locally. A typical production flow:

1. Build and push the `backend` and `frontend` images to your registry (e.g. ACR).
   Your local `docker compose up` builds these same images; production does not use
   the compose file itself.
2. Deploy the images with your platform (e.g. Kubernetes), injecting configuration:
   - non-secret values (`APP_ENV=production`, `AUTH_MODE`, URLs, flags) via a ConfigMap
   - secret values (`RESOURCE_SECRET_KEY`, `DATABASE_URL` or DB password, `ENTRA_CLIENT_SECRET`, bootstrap admin password) via a Secret
3. Point `DATABASE_URL` at your managed/external Postgres with `sslmode=require`.
   Do not run the demo Postgres container in production.
4. Set `AUTO_MIGRATE=true` so schema migrations run on startup, and leave
   `SEED_ON_START` / `RESET_DB_ON_START` at `false` (with `APP_ENV=production` these
   are enforced).
5. Provide `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD` so the first admin
   is created automatically on the initial deploy — no manual database access needed.

### Hardening (optional but recommended on Azure)

- **KEK in Key Vault.** Set `KEK_PROVIDER=azure_key_vault` with `KEK_VAULT_URL` /
  `KEK_KEY_NAME` so the key-encryption key lives in Key Vault instead of being
  derived from `RESOURCE_SECRET_KEY`. Grant the backend's identity `wrapKey` /
  `unwrapKey` ("Key Vault Crypto User") on that key. A dump plus the app's env
  then decrypts nothing without live Key Vault access.
- **Workload identity for reads.** On AKS, give the backend a workload identity
  (federated credential) that holds the Graph and Key Vault read permissions,
  and enable the "reader uses ambient identity" toggle in Administration. The
  Entra client secret then serves only Microsoft sign-in, and no vault-capable
  secret is stored in the database. The KEK unwrap uses the same identity.
- Both changes are re-wrap-safe and reversible: switching the KEK provider
  re-wraps stored data keys at startup, and the reader toggle can be turned off
  to fall back to the stored client secret.

## API surface

- Auth:
  - `GET /api/auth/bootstrap`
  - `POST /api/auth/login`
  - `POST /api/auth/logout`
  - `GET /api/auth/me`
  - `GET /api/auth/microsoft/start`
  - `GET /api/auth/microsoft/callback`
  - `POST /api/auth/password` (self-service password change)
  - `POST /api/auth/invite/accept` (set password from an invite link)
- Personal vault:
  - `GET /api/auth/vault` (status: has-vault, unlocked, methods, passkeys)
  - `POST /api/auth/vault/setup` / `POST /api/auth/vault/unlock` (passphrase)
  - `POST /api/auth/vault/passphrase` (add a passphrase to an unlocked vault)
  - `POST /api/auth/vault/passkey/setup` / `.../unlock` / `.../add` (WebAuthn PRF)
  - `POST /api/auth/vault/lock`
- Resources:
  - `GET /api/resources`
  - `POST /api/resources`
  - `GET /api/resources/{id}`
  - `PUT /api/resources/{id}`
  - `PUT /api/resources/{id}/app-registration-notifications`
  - `POST /api/resources/{id}/archive`
  - `POST /api/resources/{id}/reveal`
  - `POST /api/resources/{id}/launch`
- Key Vault:
  - `GET /api/keyvault/discover`
  - `POST /api/keyvault/import`
  - `POST /api/keyvault/sync`
- App registrations:
  - `GET /api/appregistrations/discover`
  - `POST /api/appregistrations/import`
  - `POST /api/appregistrations/sync`
- Admin:
  - `GET /api/admin/config`
  - `PUT /api/admin/config`
  - `GET /api/admin/local-groups`
  - `POST /api/admin/local-groups`
  - `PUT /api/admin/local-groups/{name}`
  - `GET /api/admin/users`
  - `POST /api/admin/users` (create; optionally issues an invite link)
  - `GET /api/admin/users/{id}` / `PUT /api/admin/users/{id}` / `DELETE /api/admin/users/{id}`
  - `POST /api/admin/users/{id}/invite` (reissue invite link)
  - `POST /api/admin/users/{id}/reset-password` (destroys the vault; issues a reset link)
  - `GET /api/admin/notification-deliveries`
  - `GET /api/admin/archived-resources`
  - `POST /api/admin/archived-resources/{id}/restore`
- Activity and audit:
  - `GET /api/audit`
  - `GET /api/me/activity`
  - `GET /api/me/notifications`
  - `POST /api/me/notifications/{id}/read`

## Data and security notes

- All deployment secrets are supplied through the environment; nothing sensitive is committed to the repository or baked into the container image. See [Configuration](#configuration).

### Encryption at rest

- **Envelope encryption.** Every stored secret has its own random data key (DEK) that encrypts the value (AES-256-GCM); the DEK is then wrapped by a key-encryption key (KEK). Rotating the KEK re-wraps data keys rather than re-encrypting every secret.
- **Pluggable KEK provider.** With `KEK_PROVIDER=local` the KEK is derived from `RESOURCE_SECRET_KEY`. With `azure_key_vault` the KEK is an RSA key that lives inside Key Vault and never leaves it — wrap/unwrap happen in the vault, authenticated by the deployment's ambient Azure identity. In that mode a database dump plus every copy of `RESOURCE_SECRET_KEY` still decrypts nothing without live, auditable Key Vault access. Existing rows re-wrap automatically at startup when the provider changes (and back, if reverted).
- **Secret classes.** Shared/team secrets and app-scope operational credentials wrap under the org KEK. Personal secrets are sealed to the owner's per-user vault public key and are unreadable by the server, admins, or a database operator — only the owner's unlocked session can decrypt them. See [Personal passwords](#personal-passwords-per-user-vault).
- **What else is protected.** Sensitive Administration settings (Entra client secret, SMTP password, RDP signing material) are encrypted at rest with the same scheme; session and browser-extension tokens are stored only as SHA-256 hashes, so a database dump exposes no usable bearer tokens.
- Use a unique `RESOURCE_SECRET_KEY` per deployment — the backend refuses to start without one, or with the legacy shared development key.

### Storage and secret modes

- Resource metadata and secret material are stored separately in `resources` and `resource_secrets`.
- The app supports `inline`, `external_reference`, and `prompt_on_launch` secret modes. Inline secret values are always encrypted (envelope scheme above) before being stored in PostgreSQL.
- Key Vault-backed resources keep metadata locally, but fetch the live secret value from Azure on demand.
- Empty `allowed_groups` means the resource is visible to everyone who can access that category.
- Launch actions return structured payloads rather than native OS execution.
- Audit events are stored in PostgreSQL and exposed through admin and per-user views.
- Key Vault sync can update metadata, auto-import discovered secrets when enabled, and archive a workspace record only after a direct Key Vault lookup confirms the object is gone.
- App registration sync stores credential expiry metadata and owner snapshots, but never stores client secret values.
- App registration records summarize the next credential expiry on the resource while retaining the full synced secret/certificate snapshot for detail views and future notifications.
- App registration discovery uses the Azure / Entra admin connection. The configured backend app registration must have Microsoft Graph application permission `Application.Read.All` with admin consent.
- App registration reminder recipients are resolved from the local owner user and the local owner team members, not from Azure owners.
- Global app registration notification defaults are managed in Administration and can be overridden per app registration or per synced credential.
- Email reminders use SMTP settings from Administration. In-app reminders are always stored in PostgreSQL for the workspace notification center.
- Administration now also shows a recent email delivery log for app registration reminders, including failed SMTP attempts and their error text.
- App registration automatic sync is configured separately in Administration and controls how often imported app registrations are rechecked for expiry changes and reminder generation.
- Connection passwords can be app-managed and encrypted locally, but Key Vault-backed secrets remain provider-owned and are still fetched live from Azure rather than duplicated into the database.
- The current Connections architecture is launcher-first: the web app owns the catalog, permissions, metadata, and launch payload generation, and the UI now attempts a custom-protocol handoff (`access-workspace://`) that the Windows launcher preview can self-register and claim on the local machine.

## Tests

Backend unit tests cover:

- auth and capability behavior
- permission filtering
- audit event logging
- archive and restore flows
- Key Vault sync and auto-import behavior
- App registration discovery, import, credential snapshot, and sync behavior
- envelope encryption: round-trip, legacy-format reads, tamper detection, KEK provider switching/re-wrap, and personal-envelope sealing

Run them from [`backend`](backend):

```powershell
go test ./...
```

Some end-to-end tests for the encryption and personal-vault flows exercise real
database behavior (migrations, startup re-encryption, invite/reset, personal↔shared
switching, passphrase unlock). They are skipped unless `VERIFY_DATABASE_URL` points
at a throwaway database:

```powershell
$env:VERIFY_DATABASE_URL="postgres://postgres:postgres@localhost:5432/verify_db?sslmode=disable"
go test ./...
```

The WebAuthn passkey (Windows Hello / Touch ID) ceremony itself cannot run headless;
its server-side crypto is unit-tested, and the browser flow is verified on an HTTPS
deployment.

## AKS-ready assumptions

- One API service
- One frontend artifact
- One PostgreSQL dependency
- The backend currently runs the lightweight automatic Key Vault sync loop in-process
- No separate worker deployment yet

## License

Released under the [MIT License](LICENSE). Copyright (c) 2026 Martin Kerhat.
