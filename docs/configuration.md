# Configuration

**Audience:** operators deploying or running the app. **Covers:** what exists today.

All configuration is read from environment variables. The application has no
baked-in secrets: secret values must be provided by the environment, and the
backend fails fast at startup if a required one is missing.

- **Local development:** copy `.env.example` to `.env` and fill in values. Docker
  Compose and the local-run instructions read from it. `.env` is gitignored; only
  `.env.example` (a template with no values) is committed.
- **Production / Kubernetes:** the same variables are injected into the container
  by the platform — non-secret values via a ConfigMap, secret values via a Secret.
  The container image itself is environment-agnostic and contains no configuration.

## Variables

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
| `ARTIFACTS_SOURCE` | no | `local` | Where downloadable builds (launcher, extensions) are listed from: `local` (a directory), `blob` (Azure Blob container), or `github` (this repo's releases). See [`artifacts/README.md`](../artifacts/README.md). |
| `ARTIFACTS_DIR` | when `local` | `/data/downloads` | Filesystem root of the artifact folders. Dev bind-mounts `./artifacts`; prod mounts a volume. |
| `ARTIFACTS_BLOB_CONTAINER_URL` / `ARTIFACTS_BLOB_SAS` | when `blob` | — | Azure Blob container URL and optional SAS token (list + read). |
| `ARTIFACTS_GITHUB_REPO` / `ARTIFACTS_GITHUB_TOKEN` | when `github` | — | Repository to serve launcher and extension builds from, and an optional token for higher rate limits. |
| `CHROME_WEB_STORE_URL` / `FIREFOX_EXTENSION_URL` | no | — | Browser-extension store listings. When set, the store becomes the primary install action and direct download the fallback. |
| `VITE_API_BASE_URL` | no (build) | dev value | Frontend build-time API base URL. |

## Runtime settings that are not environment variables

Key Vault browsing and app-registration reads authenticate either with the
Entra client secret stored in Administration, or — when the **"reader uses
ambient identity"** toggle is enabled there — with the deployment's ambient
Azure identity (workload identity / managed identity), so no vault-capable
secret needs to live in the database. That toggle is a runtime admin setting,
not an environment variable.

The same applies to Key Vault sources and their sync intervals, app registration
sync settings, SMTP delivery, expiry reminder policies, and RDP profile signing:
they are managed in Administration and stored (encrypted where sensitive) in the
database, because they are per-deployment operational settings rather than
container configuration.

## Authentication modes

The app supports two sign-in paths, selected with `AUTH_MODE`:

- `dev` — the default; reports a development label and a Microsoft-login hint to the UI
- `entra` — Microsoft sign-in backed by runtime Azure/Entra configuration from Administration

Local username/password login is available in **both** modes; `AUTH_MODE` only
changes the label and hint reported to the frontend. Every deployment therefore
has a working local sign-in for its administrator.

## Getting an initial login

Two ways to get the first user into a fresh database:

- **Bootstrap admin (recommended for real deployments):** set
  `BOOTSTRAP_ADMIN_USERNAME` and `BOOTSTRAP_ADMIN_PASSWORD`. On the first startup
  against an empty user table the app creates that admin, who can then sign in and
  configure everything else. It is idempotent — once any user exists it never runs
  again.
- **Demo seed (local/dev only):** when `SEED_ON_START=true`, the backend seeds demo
  users and sample resources. This is for local exploration and is blocked when
  `APP_ENV=production`.

Seeded demo users (only present when `SEED_ON_START=true`; password `123456`):

- `alice`: admin, groups `ops-admins`, `platform`, `engineering`
- `sam`: groups `support`, `engineering`
- `nina`: groups `network`, `platform`
- `wendy`: groups `web`, `support`
