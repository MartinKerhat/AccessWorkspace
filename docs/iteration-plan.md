# Iteration Plan

## Working model

- An AI coding assistant is the primary implementer.
- The user acts as customer and QA.
- Work is delivered in small, reviewable increments.
- Each iteration should remain testable on its own.

## Status snapshot (2026-07-23)

- Iterations 1 through 12 have been delivered in usable operational form.
- The category workspace (Connections, Key Vault, App registrations, Passwords), user/group administration, and the Azure-backed Key Vault and app registration workflows (including expiry notifications with SMTP delivery) are all live.
- Connections are a working launcher-first Windows flow: encrypted app-managed credentials, one-time launch tickets, signed RDP profiles with credential handoff and Remote Desktop Gateway support, launcher-managed SSH sessions, and a folder-tree catalog.
- The browser extension is live: one-time connect exchange, portal credential fill under policy, and silent save of new personal logins back to the workspace.
- The security foundation is delivered: all app-managed secrets use envelope encryption (deployment KEK: local key in dev, Azure Key Vault via workload identity in production); sensitive admin settings are encrypted and session tokens hashed.
- Personal secrets are cryptographically owner-only through per-user vaults: sealed to the owner's keypair, unlocked per session via login password, passphrase, or passkeys (Windows Hello / Touch ID), with a vault settings UI where users add, rename, and remove their own unlock methods.
- Sessions ride httpOnly cookies with CSRF origin checks; auth endpoints have account lockout and IP throttling; the frontend ships CSP/HSTS security headers; auth and vault events are audited.
- Passwords support saved passwords and web portal logins, including passwordless portals; personal/shared switching is owner-only and never exposes plaintext.
- The next open fronts are cross-platform launcher follow-through (Linux, then macOS), the shared cross-category expiry dashboard, and the App Configs module (see app-config-module-spec.md).

## Iteration 1: UX cleanup and product framing in UI

Status:

- completed

Objective:

Improve the current app so it better communicates the product intent, introduces a proper auth entry experience, and is easier to use.

Scope:

- add an auth entry page
- introduce an authenticated app shell structure
- keep dev/demo sign-in available for local work
- make the auth flow ready for future Azure/Entra SSO
- prepare the frontend for category-based workspace navigation
- fix layout and alignment issues
- improve sidebar and content structure
- make resource type presentation clearer
- make detail views more consistent
- improve action states and empty states

Acceptance criteria:

- main catalog page feels organized and readable
- the app has a clear entry point before the main workspace
- dev users can still sign in locally through the new entry flow
- the frontend structure is ready for future Microsoft sign-in
- the navigation direction is aligned with category-based views instead of one mixed catalog
- detail panel layout is consistent across resource types
- buttons and filters align correctly on common screen sizes
- QA can review the app visually without major layout confusion

Delivered notes:

- login entry screen and authenticated shell are in place
- category-based workspace navigation replaced the earlier mixed catalog direction
- Key Vault UI established the current picker/modal/button treatment now reused in admin flows

## Iteration 2: Object model and backend hardening

Status:

- largely complete

Objective:

Define category-specific object behavior and make the API/current flows more reliable.

Scope:

- implement category-aware object models for Connections, Key Vault, App registrations, and Passwords
- separate app-authored objects from externally sourced objects where relevant
- add stricter validation
- improve API error responses
- add more tests for permissions and audit behavior
- improve secret update safety

Acceptance criteria:

- each category has a defined required/optional field set
- external-source objects are not modeled as duplicated local secret truth
- invalid resource payloads are rejected clearly
- permission behavior is stable under tests
- audit events are created for all current sensitive actions

Delivered notes:

- category-specific validation and fields exist in the backend and UI
- external Key Vault records are stored as metadata plus provider reference, not duplicated secret truth
- permission and audit tests now cover more than the original baseline, including archive and sync behavior

## Iteration 3: Resource model refinement

Status:

- largely complete

Objective:

Represent the main business concepts more clearly.

Scope:

- introduce richer typing for connection, portal, secret, Key Vault, and app registration resources
- adjust database schema
- update admin forms and detail pages

Acceptance criteria:

- each main resource type has fields relevant to its real use
- UI reflects the type differences clearly

Delivered notes:

- resource cards, forms, and detail panels now distinguish connections, Key Vault secrets, app registrations, and passwords
- imported Key Vault objects keep Azure-owned fields read-only while preserving workspace metadata overlays
- app registrations are still mostly model/UI groundwork, not a delivered Azure-backed operational surface yet

## Iteration 4: Entra integration foundation

Status:

- initial delivery complete

Objective:

Move from mocked identity toward real Azure-backed access control.

Scope:

- design and implement initial sign-in integration
- resolve group membership
- preserve local development mode
- temporarily expose raw Azure groups, local groups, and resolved rights in the signed-in user bubble for QA/debugging only

Acceptance criteria:

- production-capable auth path exists
- group-based visibility can come from Azure identity
- temporary auth-debug information is visible during QA, with an explicit intent to move it out of the account bubble in a later UX pass

Delivered notes:

- login bootstrap, sign-in, sign-out, and Microsoft auth start/callback endpoints exist
- local development mode remains available for seeded-user QA
- admin-managed Entra runtime configuration and local-group mapping are in place
- owner pickers can use the current user directory, but per-user effective-access inspection and blocking are still missing

## Iteration 5: Key Vault integration foundation

Status:

- delivered and actively hardened

Objective:

Prepare real secret provider behavior.

Scope:

- provider abstraction
- Azure Key Vault provider
- fetch-on-demand reveal workflow
- richer Key Vault-oriented UI

Acceptance criteria:

- secrets can be represented as provider-backed records
- Key Vault retrieval happens on demand rather than from catalog storage
- the Key Vault section serves as the current reference design for picker controls, modal interactions, buttons, and text-field treatment that later UI work should follow for consistency

Delivered notes:

- Azure Key Vault provider-backed reveal flow is live
- admins can discover secrets, batch import them, and attach shared workspace metadata in one pass
- sync supports manual execution and backend automatic interval execution
- auto import can bring in newly discovered Key Vault secrets using admin-defined default metadata
- imported records can be archived on manual removal or restored later through the Key Vault archived view
- Key Vault records archive only on confirmed direct `404` lookups; disabled secrets stay visible as `disabled`
- empty allowed-groups selection now means `Everyone` instead of blocking visibility

## Iteration 6: User access administration

Status:

- delivered

Objective:

Give admins a trustworthy way to inspect who has access, why they have it, and whether that access should remain active.

Scope:

- admin user directory view
- user detail with local-group membership and mapped external groups
- effective rights and category-access diagnostics
- local-group assignment and removal from the user-management surface
- optional workspace block or suspend control for selected users
- local user creation from Administration for workspace-managed accounts
- audit coverage for admin user-management actions

Acceptance criteria:

- admins can open a user and see effective access clearly enough to answer "why can this person see this?"
- admins can adjust local-group membership from the user-management flow instead of reasoning only from group definitions
- blocked or suspended users can be prevented from entering the workspace when that control is enabled
- owner selection surfaces stay aligned with the same user directory used for administration
- admins can create a new local user without waiting for first Entra sign-in

## Iteration 7: App registrations operational integration

Status:

- implemented foundation

Objective:

Bring Azure app registrations into the workspace as a real operational surface for ownership, visibility, and credential-risk tracking.

Scope:

- Azure app registration discovery/import/sync
- application and credential metadata views
- owner and group overlays
- optional linking to related Key Vault secrets when that relationship is known
- audit coverage for app registration import actions

Acceptance criteria:

- admins can browse relevant app registrations in the workspace instead of treating the section as future placeholder UI
- app registration records carry synced owner snapshots and credential expiry metadata for later notification workflows
- app registration access follows the same rights-aware model as the rest of the workspace

## Iteration 8: Expiry tracking

Status:

- queued after the app registration integration foundation

Objective:

Add one shared operational visibility layer for expiring credentials across both Key Vault secrets and app registration credentials.

Scope:

- expirable item model
- API endpoints and views
- dashboard indicators
- severity buckets and owner visibility
- shared expiry filtering across categories
- notification/reminder support after the base visibility slice works

Acceptance criteria:

- expiring Key Vault and app registration items are visible, filterable, and linked to owners/resources
- archived items do not appear in the active expiry list
- disabled Key Vault items remain understandable in expiry views rather than disappearing mysteriously
- the expiry surface feels general rather than hard-coded to a single category

## Iteration 9: Connections foundation

Status:

- delivered as a Windows launcher MVP

Objective:

Define and begin implementing a launcher-first RDP/SSH replacement that keeps the web app as the control plane and prepares for a later cross-platform desktop helper.

Scope:

- RDP and SSH connection model refinement
- folder-path organization
- encrypted local credential storage for app-managed connection secrets
- launcher-ready launch payload metadata with a browser-side custom-protocol launcher handoff flow
- one-time backend launch tickets so connection secrets do not ride inside the browser URI
- Windows launcher support for RDP credential handoff through `cmdkey` plus visible SSH terminal launch
- local launcher status/version handshake so the web app can detect missing or outdated launcher builds before connect
- documentation for localhost-bridge launcher architecture
- initial UI detail/form support for the agreed connection fields

Acceptance criteria:

- Connections records can carry the required RDP/SSH launch metadata without abusing generic placeholder fields
- Connections catalog groups entries by stored folder path so Royal TS-style organization starts to show up in the app
- local app-managed connection passwords are encrypted before storage
- the app has a documented and testable path from connection record to local launcher execution without exposing secrets in the browser handoff

Delivered notes:

- shared RDP and SSH records now have category-specific fields, folder organization, encrypted app-managed secrets, and launcher-ready launch payloads
- the backend issues one-time launch tickets so browser-side launcher handoff does not expose decrypted connection secrets
- Windows launcher version detection and localhost bridge status checks are in place before connect
- Windows RDP launch now supports temporary credential handoff, signed stable `.rdp` profiles, trusted publisher installation, and fullscreen primary-monitor baseline behavior owned by the launcher
- Windows SSH launch now supports launcher-managed password sessions with terminal rendering that is good enough for normal shell and TUI use in current QA

## Iteration 10: Personal password objects and connection overrides

Status:

- delivered

Objective:

Add the first user-owned Password object flow and let users attach one of those saved credentials as a personal override on top of an existing shared Connection.

Scope:

- introduce personal Password objects in the Passwords category
- keep shared/default Connection username and password fields exactly where they are today
- add a per-user Connection override record that references one saved Password object
- resolve Connection launch credentials as `personal override -> connection shared/default`
- keep Connection host, port, domain, and launcher behavior owned by the Connection and launcher layers
- avoid duplicating username/password strings into another override table

Acceptance criteria:

- a user with Passwords edit rights can create a personal saved-password object without being a workspace admin
- personal Password objects are visible only to their creator
- shared Password objects remain admin-managed and keep current category behavior
- a user can assign one saved Password object as their own override for an SSH or RDP Connection
- RDP and SSH launch flows use the override username/password when present, otherwise the shared Connection credentials
- the override stores only a reference to the saved Password object, not copied secret material

Delivered notes:

- Passwords now support a simplified saved-credential flow with personal visibility for normal users and shared visibility for admin-managed objects
- personal Password objects remain private to their creator and can be reused as a per-user credential override on shared RDP and SSH Connections
- Connection launch resolution now follows `personal override -> connection shared/default` without copying username/password strings into a second override secret store
- access control was tightened so non-admin users can create only personal Password objects, password-override APIs enforce ownership, and password discovery/reveal rights stay aligned with the current category permission model

## Iteration 11: Browser extension and web portal logins

Status:

- delivered

Objective:

Bring assisted portal credential workflows into the browser while keeping the web app as the control plane.

Scope:

- extension connect flow with one-time exchange tokens and a dedicated extension session
- credential fill on allowed portals under per-object fill/reveal policy
- saving new personal logins from the browser back to the workspace
- web portal login objects with browser launch, including passwordless portals (SSO / emailed code)
- audit coverage for fill and extension actions

Delivered notes:

- the extension authenticates without ever seeing the web session cookie model change underneath it (bearer session kept through the httpOnly-cookie migration)
- personal saves work silently from any session because personal secrets encrypt to the owner's vault public key

## Iteration 12: Security foundation — encryption, personal vaults, hardening

Status:

- delivered

Objective:

Make a database dump worthless, personal secrets owner-only even against admins, and the auth perimeter resistant to brute force.

Scope:

- envelope encryption for all app-managed secrets with pluggable KEK providers (local dev key; Azure Key Vault via workload identity in production)
- encrypted sensitive admin settings; hashed session tokens
- per-user personal vaults (keypair; save-anywhere, unlock-to-read) with unlock methods: login password, passphrase, passkeys (Windows Hello / Touch ID)
- vault settings UI for user-managed unlock methods: add passphrase, add per-device passkeys with nicknames, rename, remove (last-method and login-password guards)
- account lifecycle: invites, self-service password change with vault rewrap, admin reset with explicit vault destruction
- httpOnly cookie sessions, CSRF origin checks, account lockout, IP throttling, CSP/HSTS headers
- audit expansion: login/logout, vault setup/unlock/lock, unlock-method add/remove

Delivered notes:

- personal↔shared switching rewraps keys server-side, restricted to the object owner
- recovery is by design limited to the user's own unlock methods; there are no recovery codes, and the vault settings UI warns single-method users to add a backup

## QA flow per iteration

1. The assistant implements a focused slice and verifies it locally (build, tests, typecheck).
2. The user reviews the working tree, runs and tests the behavior, and commits when satisfied.
3. The user returns bugs, change requests, or product clarifications.
4. The assistant fixes and continues to the next iteration.

## Definition of done for an iteration

- functionality works locally
- basic documentation is updated if scope changed
- tests are added or adjusted where relevant
- the result is ready for QA review

## Recommended next slice

1. cross-platform launcher follow-through, Linux first, then macOS
2. shared cross-category expiry dashboard on top of the delivered notification plumbing
3. App Configs module MVP 1 (see app-config-module-spec.md)
