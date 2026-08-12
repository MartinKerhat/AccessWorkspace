# Domain Model

**Audience:** contributors. **Covers:** the objects that exist today and how
access to them is decided.

## Overview

The product behaves as one workspace with multiple object categories. Implementation shares a base record, but the user experience and the domain model are organized around category-specific object structures.

Category set:

- `Connections`
- `Key Vault`
- `App registrations`
- `Passwords`
- `Activity`
- `Admin`

The first four are object categories. `Activity` and `Admin` are workspace areas rather than business object types.

Detailed attribute, storage, and source-of-truth rules are in [object-model.md](object-model.md).

## Core entity: Resource

A resource is a discoverable operational access item that users may be allowed to view, launch, reveal, copy, or later autofill.

Common fields:

- `id`
- `name`
- `resource_kind`
- `description`
- `owner`
- `owner_team`
- `environment`
- `allowed_groups`
- `allowed_users`
- `status`
- `created_at`
- `updated_at`
- `archived_at`

Access rule:

- sharing is expressed as local groups (`allowed_groups`), specific users (`allowed_users`), or any combination — matching either list grants visibility
- empty `allowed_groups` and `allowed_users` means the resource is visible to everyone who can access that category

Common capabilities:

- `view_allowed`
- `reveal_allowed`
- `copy_allowed`
- `launch_allowed`
- `fill_allowed` later

## Category definitions

### Connections

Used for SSH and RDP-style access.

Examples:

- SSH bastion
- Linux admin node
- RDP jump host

Core attributes:

- `connection_type` (`ssh`, `rdp`)
- `name`
- `target_host`
- `target_port`
- `username`
- `owner`
- `allowed_groups`
- `launch_allowed`
- `reveal_allowed`
- `secret_ref`
- `notes`

Additional rule:

- shared/default Connection credentials stay on the Connection object
- per-user overrides are separate references to Password objects and are resolved only at launch time

### Key Vault

Used for Azure Key Vault-backed operational objects. Today this starts with secrets, but the category should stay broad enough for future Key Vault object growth.

Examples:

- Azure Key Vault application secret
- environment secret reference
- operational secret entry from a specific vault

Core attributes:

- `name`
- `vault_name`
- `object_type` initially `secret`
- `object_name`
- `version` optional
- `uri`
- `owner`
- `allowed_groups`
- `status` such as `active` or `disabled`
- `reveal_allowed`
- `last_synced_at`
- `expires_at`
- `content_type`

Lifecycle notes:

- the workspace stores metadata and policy overlays, not the live Key Vault secret value
- a Key Vault record may be archived as a workspace soft delete while the Azure system of record remains separate
- sync should archive a Key Vault record only after a direct object lookup confirms it is gone, not merely because it disappeared from a broader discovery listing
- a disabled Key Vault secret should remain visible as `disabled` rather than being treated as deleted

### App registrations

Used for Azure app registrations and similar credential-bearing integrations.

Examples:

- Autodesk client application
- Atlassian integration app
- internal service principal

Core attributes:

- `name`
- `provider`
- `application_id`
- `tenant_id`
- `owner`
- `allowed_groups`
- `credential_type`
- `credential_expires_at`
- `owning_team`
- `linked_secret_ref`

### Passwords

Used for login/password style access to websites, tools, and legacy systems — both shared and personal.

Object shapes:

- saved password: a reusable username/password pair
- web portal login: a saved password plus a portal URL and launch/fill behavior; supports passwordless portals (SSO or emailed-code sign-in, where only URL and username are stored)

Examples:

- shared website login
- legacy admin portal credential
- a user's own saved login for a shared endpoint

Core attributes:

- `name`
- `target_url` optional
- `target_system`
- `username`
- `secret_ref`
- `owner`
- `allowed_groups`
- `launch_allowed` for web portal logins
- `reveal_allowed`
- `copy_allowed`
- `notes`
- `personal`

Additional rules:

- personal Password objects are visible only to their creator and encrypted to that user's personal vault (see Secret model)
- these Password objects may be reused as per-user overrides for shared SSH and RDP Connections
- switching an object between personal and shared is an owner-only action and never exposes the plaintext

## Secret model

Secret handling stays separate from resource metadata.

### Secret modes

- `inline` — app-managed value, stored under envelope encryption
- `external_reference` — pointer to a value owned elsewhere
- `azure_key_vault` — fetched on demand from Key Vault, never persisted locally
- `prompt_on_launch` — connections that ask for the credential at launch time
- `none` — passwordless web portal logins

### Encryption classes (app-managed values)

Every stored value is envelope-encrypted (per-secret data key, wrapped by a deployment KEK — local key in development, Azure Key Vault via workload identity in production). The wrap differs by class:

- `shared` — readable by authorized users under category policy
- `personal` — sealed to the owner's personal vault public key; reading requires the owner's unlocked session, so admins and database access cannot decrypt it
- `app-scope` — integration credentials (Entra, SMTP, RDP signing) the backend needs without a user session

### Personal vault

Each user has a vault keypair. Saving encrypts to the public key (no unlock needed, from any session); reading requires the private key, which is unlocked per session by one of the user's methods: local login password (automatic at sign-in), passphrase, or passkeys (Windows Hello / Touch ID, one per device). Users manage their methods — add, rename, remove — from the vault settings UI. Losing all methods makes the vault unrecoverable by design.

Rule:

- metadata lives in catalog records
- actual secret retrieval happens through providers or decryption on demand, and is audited

## Identity and authorization model

### User

Current state:

- Entra-backed sign-in and local workspace accounts coexist; local accounts support invites, self-service password change, and admin reset (which destroys the personal vault by design)

Fields:

- `user_id`
- `display_name`
- `email`
- `group_ids`
- `roles`

### Group access

Resources should be visible based primarily on Azure/Entra groups. The app may also have app-local admin roles, but group mapping should remain the main control plane.

Practical rule:

- local groups and resolved external groups both participate in visibility
- a resource may additionally be shared with specific users directly (`allowed_users`), independent of group membership
- a resource with no allowed groups and no allowed users is intentionally shared to everyone with category access

### Menu visibility

Workspace navigation should be derived from rights.

Suggested rule:

- show a category if the user has access to at least one object in that category
- show `Activity` for every signed-in user
- show `Admin` only for users with admin capability

## Audit model

Sensitive actions audited today:

- resource viewed / revealed / copied / launched / filled
- resource created / updated / archived / deleted / restored
- login succeeded / failed, logout
- vault setup / unlocked / locked
- vault unlock method added / removed
- admin user-management actions

Suggested audit fields:

- `event_id`
- `event_type`
- `user_id`
- `resource_id`
- `resource_kind`
- `metadata`
- `created_at`

## Expiry model

Expiry is tracked as operational data on the object itself, and drives
reminders rather than a separate dashboard.

Two sources produce expiring items:

- **app registration credentials** — every synced secret and certificate, each
  with its own expiry; the resource also carries a summary of the next one to
  expire
- **Key Vault secrets** — the expiry of the version the workspace record points
  at (`expires_at`, from the Azure `exp` attribute). Most secrets carry none, and
  those never produce a reminder

Both reduce to the same shape for reminder purposes: an owning resource, an item
key, a display name, a kind (`secret` / `certificate`), and an expiry date.

**Status.** An object's expiry maps onto the shared status vocabulary used by
the catalog badge — `expired`, `expiring` (within 30 days), or `active`. For Key
Vault secrets `disabled` takes precedence, because an unusable secret is the
more urgent fact.

**Reminder policy.** Each category has a global policy (enabled, reminder days,
channels) managed in Administration. App registrations additionally support a
per-application and a per-credential override. Reminders are delivered in the
notification center and by email, to the object's owner, the owner team, and
workspace admins. Reminders written for an expiry date that no longer applies —
after a rotation, or when a credential is deleted — are retired automatically.

## Import source

Records carry a source kind that distinguishes app-authored objects from
records imported and kept in sync from an external system, which determines
which fields the workspace owns and which it must not overwrite. See
[object-model.md](object-model.md).
