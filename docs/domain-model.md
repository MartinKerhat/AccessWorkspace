# Domain Model

## Overview

The product should behave as one workspace with multiple object categories. It may still use a shared base record in implementation, but the user experience and domain model should be organized around category-specific object structures.

Initial category set:

- `Connections`
- `Key Vault`
- `App registrations`
- `Passwords`
- `Activity`
- `Admin`

The first four are object categories. `Activity` and `Admin` are workspace areas rather than business object types.

Detailed attribute, storage, and source-of-truth rules are defined in [object-model-spec.md](object-model-spec.md).

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
- `status`
- `created_at`
- `updated_at`
- `archived_at`

Access rule:

- empty `allowed_groups` means the resource is visible to everyone who can access that category

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
- `launch_profile`
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

Used for shared login/password style access to websites, tools, and legacy systems.

Examples:

- shared website login
- legacy admin portal credential
- shared vendor account

Core attributes:

- `name`
- `target_url` optional
- `target_system`
- `username`
- `secret_ref`
- `owner`
- `allowed_groups`
- `reveal_allowed`
- `copy_allowed`
- `notes`
- `personal`

Additional rule:

- personal Password objects are visible only to their creator
- these Password objects may be reused as per-user overrides for shared SSH and RDP Connections

## Secret model

Secret handling should remain separate from resource metadata.

### Secret record

Fields:

- `secret_id`
- `provider_type`
- `inline_value` for local development only
- `external_reference`
- `display_hint`
- `expires_at`
- `last_checked_at`

### Provider types

- `inline`
- `azure_key_vault`
- `external_reference`

Target rule:

- metadata lives in catalog records
- actual secret retrieval happens through providers on demand

## Identity and authorization model

### User

Near term:

- local development identity

Target:

- Entra-backed user identity

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
- a resource with no allowed groups is intentionally shared to everyone with category access

### Menu visibility

Workspace navigation should be derived from rights.

Suggested rule:

- show a category if the user has access to at least one object in that category
- show `Activity` for every signed-in user
- show `Admin` only for users with admin capability

## Audit model

Sensitive actions to audit:

- resource viewed
- resource revealed
- secret copied
- resource launched
- portal fill requested later
- resource created
- resource updated
- resource archived
- resource restored

Suggested audit fields:

- `event_id`
- `event_type`
- `user_id`
- `resource_id`
- `resource_kind`
- `metadata`
- `created_at`

## Expiry model

The app should track expirations as first-class operational data.

### Expirable item

Fields:

- `item_id`
- `item_type`
- `resource_id`
- `provider`
- `display_name`
- `owner`
- `expires_at`
- `severity`
- `status`
- `last_checked_at`

Item types:

- app registration credential
- shared secret
- Key Vault secret metadata
- portal credential if applicable

## Future supporting entities

### Launch profile

Defines how a local helper should open a resource on each platform.

### Import source

Tracks whether a record is manual, synced, or partially synced from external systems.

### Reminder rule

Controls how and when expiring items are surfaced or notified.
