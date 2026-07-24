# Roadmap

## Goal

Deliver Access Workspace in small, testable steps that move from a local catalog MVP toward a production-ready internal access platform.

## Phase 0: Baseline MVP

Status:

- completed as a first technical baseline

Delivered:

- backend API
- frontend catalog shell
- PostgreSQL schema and seed data
- dev auth mode
- audit logging basics
- Docker local setup

Known gaps:

- rough UI and layout consistency
- mixed catalog navigation model
- generic resource model
- only baseline auth flow at this stage
- only baseline secret placeholders at this stage
- no expiry features
- no local launcher helper

Historical note:

- the Azure/Entra and Key Vault gaps listed above were addressed in later phases and are kept here only to describe the original baseline state

## Phase 1: Product and UX stabilization

Status:

- completed

Goal:

Make the current MVP feel coherent, safer to use, and closer to the intended category-based workspace.

Scope:

- add an auth entry page and authenticated app shell
- preserve development login flow in a cleaner form
- clean up layout and information hierarchy
- replace the default mixed catalog direction with category-based navigation planning
- improve resource cards and detail views by type
- make admin workflows clearer
- improve validation and error handling
- strengthen backend and frontend tests

Exit criteria:

- workspace structure is easier to scan and act on
- the frontend has a clear login-entry structure for future SSO
- resource types are visually distinct
- core flows are stable enough for repeated QA rounds

Delivered highlights:

- authenticated app shell and login entry screen
- category-based workspace navigation
- clearer type-specific views and admin workflows

## Phase 2: Domain model refinement

Status:

- largely complete

Goal:

Model connections, Key Vault, app registrations, and passwords more explicitly.

Scope:

- refine resource schema
- add ownership and lifecycle fields
- add expirable entity model
- define provider-oriented secret references

Exit criteria:

- catalog records match the real business concepts better
- backend schema is ready for integration work

Delivered highlights:

- category-specific resource forms and detail surfaces
- source-aware resource modeling for manual and external records
- stronger validation and richer audit coverage
- app registrations are modeled in the product shape, but real Azure app registration integration is still pending

## Phase 3: Azure/Entra integration

Status:

- initial delivery complete

Goal:

Use real identity and group membership.

Scope:

- Entra login
- Azure group resolution
- group-based visibility and role mapping
- dev fallback retained for local development

Exit criteria:

- the app respects real user identity and group membership

Delivered highlights:

- Microsoft sign-in start/callback flow
- admin-managed Entra configuration
- local-group mapping and category capability evaluation
- dev fallback retained for local development and QA
- per-user effective-access inspection and workspace-level user blocking are still missing and move to the next phase

## Phase 4: Key Vault operational UI

Status:

- initial delivery complete and under active refinement

Goal:

Make Access Workspace a better operational interface for Key Vault.

Scope:

- Key Vault provider integration
- searchable secret metadata views
- reveal on demand under policy
- clearer secret-focused UX
- audit reveal and related actions
- import, sync, and restore workflows

Exit criteria:

- users can work with approved Key Vault secrets faster than in Azure Portal for common tasks

Delivered highlights:

- Key Vault secret discovery and batch import
- reveal-on-demand without storing the live Key Vault secret locally
- manual sync plus backend automatic interval sync
- optional auto import with default owner/team/group metadata
- archived Key Vault view with restore
- disabled secrets remain visible as disabled; confirmed missing secrets can be archived from the workspace catalog

## Phase 5: User access administration

Status:

- delivered

Goal:

Give admins a reliable way to inspect, explain, and govern user access inside the workspace.

Scope:

- admin user directory
- effective category and action rights per user
- visibility into local groups, mapped external groups, and resolved access
- local-group assignment/removal from a user-centered workflow
- optional workspace user block or suspension control
- local workspace user creation from Administration
- audit for admin user-management actions

Exit criteria:

- admins can answer who has access and why without reconstructing it manually from group definitions
- admins can adjust user access from the administration surface without editing only group objects
- blocked users can be prevented from entering the workspace when that control is enabled
- admins can create local workspace users directly from the UI

## Phase 6: App registrations operational integration

Status:

- delivered, including credential-expiry notifications (in-app notification center + SMTP email with delivery log and per-resource policy overrides)

Goal:

Bring Azure app registrations into the workspace as a real operational surface for ownership and credential-risk tracking.

Scope:

- Azure app registration discovery/import/sync
- application and credential metadata views
- owner and group overlays
- credential metadata needed for expiry monitoring and later notification workflows
- optional linking to related Key Vault secrets when that relationship is known

Exit criteria:

- admins can browse relevant app registrations in the workspace
- credential metadata is visible enough to support later shared expiry and notification workflows

## Phase 7: Expiry visibility

Status:

- partially covered: app registration credential expiry with reminders/notifications is live; the shared cross-category expiry dashboard remains open

Goal:

Reduce outages and missed renewals from expiring credentials with one shared system that covers both Key Vault secrets and app registration credentials.

Scope:

- expiring items model
- expiry views and dashboard widgets
- severity and owner visibility
- reminder logic and scheduling support
- shared cross-category expiry filtering and ownership views

Exit criteria:

- expiring Key Vault secrets and app registration credentials are visible and actionable through one coherent workflow

## Phase 8: Connections launcher-first replacement

Status:

- initial Windows MVP delivered

Goal:

Replace Royal TS usage for shared RDP and SSH access with one central catalog plus a cross-platform launcher path.

Scope:

- RDP and SSH connection model refinement
- folder-path organization for connection records
- encrypted locally stored connection credentials
- launcher-ready launch payload metadata
- local helper / launcher architecture using a localhost bridge
- Windows, macOS, and Linux launcher targets in phased delivery

Exit criteria:

- users can manage shared RDP and SSH records in the workspace using a model aligned with launcher execution
- stored local connection passwords are encrypted at rest
- the backend and UI expose enough launch metadata to support a later cross-platform launcher

Delivered highlights so far:

- RDP and SSH records are now modeled as real connection objects with folder organization and app-managed encrypted secrets
- the web app performs launcher version/status checks before connect and hands off through backend-issued one-time launch tickets
- the Windows launcher now provides working RDP and SSH connect flows, including trusted RDP publisher installation, signed `.rdp` profile launch, RDP credential handoff, and launcher-managed SSH password sessions
- RDP connections support Remote Desktop Gateway hosts end to end (record field, launch payload, launcher execution, reachability diagnostics)
- the Connections catalog renders folder paths as a tree for Royal TS-style organization

## Phase 9: Personal credential overlays

Status:

- delivered

Goal:

Let shared Connections keep their default service credentials while giving individual users a safe way to substitute their own saved username/password pair for the same endpoint.

Scope:

- baseline personal Password objects in the Passwords category
- per-user Connection override references
- launch-time credential resolution from saved Password objects
- UI flows for managing personal saved credentials and assigning them to Connections

Exit criteria:

- Password objects can act as reusable saved credential records
- personal Password objects remain private to their creator
- shared Connection defaults continue to work untouched when no override exists
- a user can connect to the same RDP or SSH endpoint with their own saved credentials when needed

Delivered highlights:

- Passwords now support a baseline saved-credential object that stores username/password pairs once and can be reused instead of duplicating secret values across tables
- non-admin users can create personal Password objects for themselves, while shared Password objects remain an admin-managed pattern
- each user can assign one of their own saved Password objects as a per-user override on a shared RDP or SSH Connection
- launch-time credential resolution now follows `personal override -> connection shared/default` for both RDP and SSH

## Phase 10: Connections hardening and launcher follow-through

Status:

- Linux launcher delivered (2026-07); macOS and machine-local preference work remain

Delivered highlights so far:

- the Linux launcher covers the full connect surface: RDP via the system FreeRDP client (credential handoff over stdin, Remote Desktop Gateway, admin sessions), launcher-managed SSH sessions in the user's terminal emulator, per-user XDG self-install with `access-workspace://` handler and bridge autostart, distributed as a tarball alongside the Windows build
- the launcher status endpoint now reports platform and per-feature capabilities, so the web app can tell users about missing prerequisites (e.g. FreeRDP not installed) before a launch is attempted

Goal:

Finish turning Connections into a dependable Royal TS replacement by pushing machine-local behavior into the launcher and broadening the operational model around shared endpoints.

Scope:

- launcher-owned machine-local preferences such as monitor/fullscreen behavior and later per-device connect defaults
- broader Connections administration and quality-of-life improvements around shared endpoints and folders
- stronger launcher diagnostics, versioning, and recovery workflows
- cross-platform launcher follow-through for macOS and Linux after the Windows baseline

Exit criteria:

- launcher-managed local behavior is clearly separated from shared Connection metadata owned by the web app
- Windows launcher flows remain stable without reintroducing RDP publisher trust friction or SSH regressions
- the architecture is ready for equivalent launcher behavior on macOS and Linux

## Phase 11: Browser extension

Status:

- delivered

Goal:

Improve portal access workflows.

Scope:

- extension authentication/authorization flow
- allowed-site matching
- approved fill workflows
- audit for sensitive extension actions

Exit criteria:

- users can use assisted fill for selected approved portals

Delivered highlights:

- extension connect flow through one-time exchange tokens and a dedicated extension session
- credential fill on allowed portals, honoring per-object fill/reveal policy
- saving new personal logins from the browser back into the workspace (silent save via the personal vault public key)
- web portal login objects with launch-in-browser support, including passwordless portals
- fill actions audited like other sensitive actions

## Phase 12: Security foundation — encryption, personal vaults, hardening

Status:

- delivered

Goal:

Close the passive and active security gaps of the earlier phases: no plaintext secrets at rest, personal secrets unreadable even by admins, and a hardened session/perimeter layer.

Scope:

- envelope encryption for all app-managed secrets (per-secret data keys wrapped by a deployment KEK: local key in dev, Azure Key Vault via workload identity in production)
- encryption of sensitive admin settings and hashing of session tokens
- personal vaults: per-user keypair, personal secrets sealed to the owner, unreadable by admins or database access
- vault unlock methods: local login password (automatic), passphrase, passkeys (Windows Hello / Touch ID); user-managed add/rename/remove with last-method and login-password guards
- account lifecycle: invites, self-service password change (vault rewraps), admin reset with explicit vault destruction
- session hardening: httpOnly cookie sessions, CSRF origin checks, no tokens in localStorage or redirect URLs
- perimeter: account lockout, per-IP throttling of auth endpoints, CSP/HSTS/security headers
- audit expansion: login/logout, vault setup/unlock/lock, unlock-method changes

Exit criteria:

- a database dump alone yields no secret material and no session takeover
- personal secrets are cryptographically owner-only, with recovery by design limited to the user's own unlock methods
- brute-force attempts against login or vault unlock are throttled, locked out, and visible in the audit log

## Current focus (2026-07-23)

Open fronts, in no committed order:

- macOS launcher follow-through (Linux delivered 2026-07)
- shared cross-category expiry dashboard on top of the delivered notification plumbing
- App Configs module MVP 1 (see [app-config-module-spec.md](app-config-module-spec.md))
- remaining security follow-ups: session-revocation controls, optional second factor for local-account login

## Ongoing work across phases

- QA feedback and defect fixing
- audit coverage improvements
- documentation updates
- deployment and operational hardening
