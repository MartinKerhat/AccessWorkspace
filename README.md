# Access Workspace

Monorepo for an internal operational access workspace. It lets company users sign in through local development mode or Microsoft sign-in, browse category-based resources their groups can use, reveal approved secrets on demand, request launch payloads for SSH/RDP/web items, and inspect activity and audit history. Admins can manage local groups, configure Azure/Entra and Key Vault sources, and sync/import Key Vault metadata without duplicating live secret values locally.

## Repo layout

- `backend`: Go API, PostgreSQL migrations, seed command, and tests
- `frontend`: React + TypeScript UI
- `deploy`: Dockerfiles and deployment-oriented assets
- `docs`: product, architecture, roadmap, and QA planning docs

## Documentation

- [Product Vision](docs/product-vision.md)
- [Architecture](docs/architecture.md)
- [App Config Module Spec](docs/app-config-module-spec.md)
- [Domain Model](docs/domain-model.md)
- [Object Model Spec](docs/object-model-spec.md)
- [Roadmap](docs/roadmap.md)
- [Iteration Plan](docs/iteration-plan.md)
- [Browser Extension Distribution](docs/browser-extension-distribution.md)
- [QA Workflow](docs/qa-workflow.md)

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
- Administration user directory with effective-access inspection, direct group assignment, workspace blocking, and local user creation
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
| `RESOURCE_SECRET_KEY` | **yes** | — | AES key for encrypting inline secrets. Generate a unique value per deployment (`openssl rand -base64 32`). Startup fails if empty or set to the legacy shared dev key. |
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
| `FRONTEND_URL` | no | dev value | Frontend URL for CORS/redirects. |
| `ENTRA_TENANT_ID` / `ENTRA_CLIENT_ID` / `ENTRA_CLIENT_SECRET` | when `AUTH_MODE=entra` | — | Microsoft Entra app credentials. Startup fails if `AUTH_MODE=entra` and any are missing. |
| `ENTRA_AUTHORITY` / `ENTRA_REDIRECT_URI` / `ENTRA_GROUP_SOURCE` / `ENTRA_DIRECT_RIGHTS_JSON` | no | see `.env.example` | Additional Entra settings. |
| `ARTIFACTS_SOURCE` | no | `local` | Where downloadable builds (launcher, extensions) are listed from: `local` (a directory) or `blob` (Azure Blob container). See [`artifacts/README.md`](artifacts/README.md). |
| `ARTIFACTS_DIR` | when `local` | `/data/downloads` | Filesystem root of the artifact folders. Dev bind-mounts `./artifacts`; prod mounts a volume. |
| `ARTIFACTS_BLOB_CONTAINER_URL` / `ARTIFACTS_BLOB_SAS` | when `blob` | — | Azure Blob container URL and optional SAS token (list + read). |
| `CHROME_WEB_STORE_URL` / `FIREFOX_EXTENSION_URL` | no | — | Browser-extension store listings. When set, the store becomes the primary install action and direct download the fallback. |
| `VITE_API_BASE_URL` | no (build) | dev value | Frontend build-time API base URL. |

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

## API surface

- Auth:
  - `GET /api/auth/bootstrap`
  - `POST /api/auth/login`
  - `POST /api/auth/logout`
  - `GET /api/auth/me`
  - `GET /api/auth/microsoft/start`
  - `GET /api/auth/microsoft/callback`
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
  - `GET /api/admin/notification-deliveries`
  - `GET /api/admin/archived-resources`
  - `POST /api/admin/archived-resources/{id}/restore`
- Activity and audit:
  - `GET /api/audit`
  - `GET /api/me/activity`
  - `GET /api/me/notifications`
  - `POST /api/me/notifications/{id}/read`

## Data and security notes

- All secrets are supplied through the environment; nothing sensitive is committed to the repository or baked into the container image. See [Configuration](#configuration).
- Inline secret values are encrypted with `RESOURCE_SECRET_KEY` before being stored in PostgreSQL. Use a unique key per deployment — the backend refuses to start without one, or with the legacy shared development key.
- Resource metadata and secret material are stored separately in `resources` and `resource_secrets`.
- The app supports `inline`, `external_reference`, and `prompt_on_launch` secret modes. Inline secret values are encrypted before they are stored in PostgreSQL.
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

Run them from [`backend`](backend):

```powershell
go test ./...
```

## AKS-ready assumptions

- One API service
- One frontend artifact
- One PostgreSQL dependency
- The backend currently runs the lightweight automatic Key Vault sync loop in-process
- No separate worker deployment yet

## License

Released under the [MIT License](LICENSE). Copyright (c) 2026 Martin Kerhat.
