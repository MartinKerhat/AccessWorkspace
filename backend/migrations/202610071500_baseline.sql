-- Baseline schema (replaces migrations 001-019, applied in production as of
-- 2026-10-07). Generated from `pg_dump --schema-only` of a database that had
-- exactly those migrations applied, so a fresh database ends up identical to
-- a legacy-migrated one. Do not edit; add new timestamped migrations instead.
--
-- Deployed instances whose ledger still lists the legacy 001-019 file names
-- are recognised by the runner (see db.RunMigrations): the legacy rows are
-- replaced by this file's name without re-running anything.

CREATE TABLE public.admin_settings (
    key text NOT NULL,
    value text DEFAULT ''::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.app_registration_credentials (
    resource_id text NOT NULL,
    key_id text NOT NULL,
    credential_type text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    start_date_time timestamp with time zone,
    end_date_time timestamp with time zone,
    hint text DEFAULT ''::text NOT NULL,
    usage text DEFAULT ''::text NOT NULL,
    last_synced_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.app_registration_owners (
    resource_id text NOT NULL,
    owner_id text NOT NULL,
    owner_type text DEFAULT ''::text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    email text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.app_users (
    id text NOT NULL,
    username text NOT NULL,
    display_name text NOT NULL,
    email text NOT NULL,
    password_hash text NOT NULL,
    groups text[] DEFAULT '{}'::text[] NOT NULL,
    is_admin boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    workspace_blocked boolean DEFAULT false NOT NULL,
    direct_rights text[] DEFAULT '{}'::text[] NOT NULL,
    failed_login_attempts integer DEFAULT 0 NOT NULL,
    locked_until timestamp with time zone
);

CREATE TABLE public.audit_events (
    id text NOT NULL,
    event_type text NOT NULL,
    user_id text NOT NULL,
    user_name text NOT NULL,
    resource_id text,
    resource_name text,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.auth_sessions (
    token text NOT NULL,
    user_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    vault_private_key text DEFAULT ''::text NOT NULL,
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    last_used_at timestamp with time zone DEFAULT now() NOT NULL,
    created_ip text DEFAULT ''::text NOT NULL,
    user_agent text DEFAULT ''::text NOT NULL
);

CREATE TABLE public.browser_extension_connect_tokens (
    token text NOT NULL,
    user_id text NOT NULL,
    auth_mode text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    vault_private_key text DEFAULT ''::text NOT NULL
);

CREATE TABLE public.browser_extension_sessions (
    token text NOT NULL,
    user_id text NOT NULL,
    installation_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    vault_private_key text DEFAULT ''::text NOT NULL,
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_ip text DEFAULT ''::text NOT NULL,
    user_agent text DEFAULT ''::text NOT NULL
);

CREATE TABLE public.connection_user_password_overrides (
    connection_id text NOT NULL,
    user_id text NOT NULL,
    password_resource_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.expiry_notification_policies (
    resource_id text NOT NULL,
    credential_key_id text DEFAULT ''::text NOT NULL,
    enabled boolean NOT NULL,
    reminder_days integer[] DEFAULT '{}'::integer[] NOT NULL,
    channels text[] DEFAULT '{}'::text[] NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.expiry_notifications (
    id text NOT NULL,
    user_id text NOT NULL,
    resource_id text NOT NULL,
    resource_name text DEFAULT ''::text NOT NULL,
    credential_key_id text NOT NULL,
    credential_display_name text DEFAULT ''::text NOT NULL,
    credential_type text DEFAULT ''::text NOT NULL,
    credential_end_date_time timestamp with time zone,
    reminder_day integer NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    channels text[] DEFAULT '{}'::text[] NOT NULL,
    read_at timestamp with time zone,
    email_status text DEFAULT ''::text NOT NULL,
    email_sent_at timestamp with time zone,
    email_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.local_groups (
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    rights text[] DEFAULT '{}'::text[] NOT NULL,
    mapped_external_groups text[] DEFAULT '{}'::text[] NOT NULL,
    assigned_user_ids text[] DEFAULT '{}'::text[] NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.resource_secrets (
    resource_id text NOT NULL,
    secret_mode text NOT NULL,
    secret_value text DEFAULT ''::text NOT NULL,
    secret_reference text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.resources (
    id text NOT NULL,
    name text NOT NULL,
    type text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    owner text DEFAULT ''::text NOT NULL,
    target_host text DEFAULT ''::text NOT NULL,
    target_port integer,
    username text DEFAULT ''::text NOT NULL,
    launch_allowed boolean DEFAULT false NOT NULL,
    reveal_allowed boolean DEFAULT false NOT NULL,
    allowed_groups text[] DEFAULT '{}'::text[] NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    archived_at timestamp with time zone,
    owner_team text DEFAULT ''::text NOT NULL,
    environment text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    source_kind text DEFAULT 'manual'::text NOT NULL,
    source_object_id text DEFAULT ''::text NOT NULL,
    last_synced_at timestamp with time zone,
    notes text DEFAULT ''::text NOT NULL,
    target_url text DEFAULT ''::text NOT NULL,
    target_system text DEFAULT ''::text NOT NULL,
    vault_name text DEFAULT ''::text NOT NULL,
    object_name text DEFAULT ''::text NOT NULL,
    object_type text DEFAULT ''::text NOT NULL,
    object_version text DEFAULT ''::text NOT NULL,
    content_type text DEFAULT ''::text NOT NULL,
    expires_at timestamp with time zone,
    provider text DEFAULT ''::text NOT NULL,
    application_id text DEFAULT ''::text NOT NULL,
    tenant_id text DEFAULT ''::text NOT NULL,
    client_id text DEFAULT ''::text NOT NULL,
    credential_type text DEFAULT ''::text NOT NULL,
    credential_expires_at timestamp with time zone,
    display_name_external text DEFAULT ''::text NOT NULL,
    linked_secret_ref text DEFAULT ''::text NOT NULL,
    copy_allowed boolean DEFAULT false NOT NULL,
    folder_path text DEFAULT ''::text NOT NULL,
    launch_mode text DEFAULT ''::text NOT NULL,
    connection_domain text DEFAULT ''::text NOT NULL,
    connection_admin_session boolean DEFAULT false NOT NULL,
    connection_automatic_logon boolean DEFAULT false NOT NULL,
    connection_window_mode text DEFAULT ''::text NOT NULL,
    connection_use_multiple_monitors boolean DEFAULT false NOT NULL,
    connection_show_connection_bar boolean DEFAULT true NOT NULL,
    connection_screen_mode text DEFAULT ''::text NOT NULL,
    connection_mac_address text DEFAULT ''::text NOT NULL,
    personal boolean DEFAULT false NOT NULL,
    owner_user_id text DEFAULT ''::text NOT NULL,
    connection_gateway_host text DEFAULT ''::text NOT NULL,
    allowed_users text[] DEFAULT '{}'::text[] NOT NULL
);

CREATE TABLE public.user_invites (
    token text NOT NULL,
    user_id text NOT NULL,
    purpose text DEFAULT 'invite'::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL
);

CREATE TABLE public.user_vault_unlocks (
    user_id text NOT NULL,
    method text NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    wrapped_private_key text NOT NULL,
    salt text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    nickname text DEFAULT ''::text NOT NULL
);

CREATE TABLE public.user_vaults (
    user_id text NOT NULL,
    public_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.admin_settings
    ADD CONSTRAINT admin_settings_pkey PRIMARY KEY (key);

ALTER TABLE ONLY public.app_registration_credentials
    ADD CONSTRAINT app_registration_credentials_pkey PRIMARY KEY (resource_id, key_id, credential_type);

ALTER TABLE ONLY public.expiry_notification_policies
    ADD CONSTRAINT app_registration_notification_policies_pkey PRIMARY KEY (resource_id, credential_key_id);

ALTER TABLE ONLY public.expiry_notifications
    ADD CONSTRAINT app_registration_notification_user_id_resource_id_credentia_key UNIQUE (user_id, resource_id, credential_key_id, credential_end_date_time, reminder_day);

ALTER TABLE ONLY public.expiry_notifications
    ADD CONSTRAINT app_registration_notifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.app_registration_owners
    ADD CONSTRAINT app_registration_owners_pkey PRIMARY KEY (resource_id, owner_id);

ALTER TABLE ONLY public.app_users
    ADD CONSTRAINT app_users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.app_users
    ADD CONSTRAINT app_users_username_key UNIQUE (username);

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_pkey PRIMARY KEY (token);

ALTER TABLE ONLY public.browser_extension_connect_tokens
    ADD CONSTRAINT browser_extension_connect_tokens_pkey PRIMARY KEY (token);

ALTER TABLE ONLY public.browser_extension_sessions
    ADD CONSTRAINT browser_extension_sessions_pkey PRIMARY KEY (token);

ALTER TABLE ONLY public.connection_user_password_overrides
    ADD CONSTRAINT connection_user_password_overrides_pkey PRIMARY KEY (connection_id, user_id);

ALTER TABLE ONLY public.local_groups
    ADD CONSTRAINT local_groups_pkey PRIMARY KEY (name);

ALTER TABLE ONLY public.resource_secrets
    ADD CONSTRAINT resource_secrets_pkey PRIMARY KEY (resource_id);

ALTER TABLE ONLY public.resources
    ADD CONSTRAINT resources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_invites
    ADD CONSTRAINT user_invites_pkey PRIMARY KEY (token);

ALTER TABLE ONLY public.user_vault_unlocks
    ADD CONSTRAINT user_vault_unlocks_pkey PRIMARY KEY (user_id, method, label);

ALTER TABLE ONLY public.user_vaults
    ADD CONSTRAINT user_vaults_pkey PRIMARY KEY (user_id);

CREATE INDEX idx_app_registration_credentials_end_date_time ON public.app_registration_credentials USING btree (end_date_time);

CREATE INDEX idx_app_registration_credentials_resource_id ON public.app_registration_credentials USING btree (resource_id);

CREATE INDEX idx_app_registration_owners_resource_id ON public.app_registration_owners USING btree (resource_id);

CREATE INDEX idx_audit_events_resource_id ON public.audit_events USING btree (resource_id);

CREATE INDEX idx_audit_events_user_id ON public.audit_events USING btree (user_id);

CREATE INDEX idx_auth_sessions_expires_at ON public.auth_sessions USING btree (expires_at);

CREATE UNIQUE INDEX idx_auth_sessions_id ON public.auth_sessions USING btree (id);

CREATE INDEX idx_auth_sessions_user_id ON public.auth_sessions USING btree (user_id);

CREATE INDEX idx_browser_extension_connect_tokens_expires_at ON public.browser_extension_connect_tokens USING btree (expires_at);

CREATE INDEX idx_browser_extension_connect_tokens_user_id ON public.browser_extension_connect_tokens USING btree (user_id);

CREATE INDEX idx_browser_extension_sessions_expires_at ON public.browser_extension_sessions USING btree (expires_at);

CREATE UNIQUE INDEX idx_browser_extension_sessions_id ON public.browser_extension_sessions USING btree (id);

CREATE UNIQUE INDEX idx_browser_extension_sessions_installation_id ON public.browser_extension_sessions USING btree (installation_id);

CREATE INDEX idx_browser_extension_sessions_user_id ON public.browser_extension_sessions USING btree (user_id);

CREATE INDEX idx_connection_user_password_overrides_user_id ON public.connection_user_password_overrides USING btree (user_id);

CREATE INDEX idx_expiry_notifications_created_at ON public.expiry_notifications USING btree (created_at DESC);

CREATE INDEX idx_expiry_notifications_pending_email ON public.expiry_notifications USING btree (user_id, reminder_day) WHERE ((read_at IS NULL) AND (email_status <> 'sent'::text));

CREATE INDEX idx_expiry_notifications_read_at ON public.expiry_notifications USING btree (read_at);

CREATE INDEX idx_expiry_notifications_user_id ON public.expiry_notifications USING btree (user_id);

CREATE INDEX idx_resources_allowed_groups ON public.resources USING gin (allowed_groups);

CREATE INDEX idx_resources_allowed_users ON public.resources USING gin (allowed_users);

CREATE INDEX idx_resources_archived_at ON public.resources USING btree (archived_at);

CREATE INDEX idx_resources_folder_path ON public.resources USING btree (folder_path);

CREATE INDEX idx_resources_owner_user_id ON public.resources USING btree (owner_user_id);

CREATE INDEX idx_resources_personal ON public.resources USING btree (personal);

CREATE INDEX idx_resources_type ON public.resources USING btree (type);

CREATE INDEX idx_user_invites_expires_at ON public.user_invites USING btree (expires_at);

CREATE INDEX idx_user_invites_user_id ON public.user_invites USING btree (user_id);

ALTER TABLE ONLY public.app_registration_credentials
    ADD CONSTRAINT app_registration_credentials_resource_id_fkey FOREIGN KEY (resource_id) REFERENCES public.resources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.expiry_notification_policies
    ADD CONSTRAINT app_registration_notification_policies_resource_id_fkey FOREIGN KEY (resource_id) REFERENCES public.resources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.expiry_notifications
    ADD CONSTRAINT app_registration_notifications_resource_id_fkey FOREIGN KEY (resource_id) REFERENCES public.resources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.expiry_notifications
    ADD CONSTRAINT app_registration_notifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.app_users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.app_registration_owners
    ADD CONSTRAINT app_registration_owners_resource_id_fkey FOREIGN KEY (resource_id) REFERENCES public.resources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.app_users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.browser_extension_connect_tokens
    ADD CONSTRAINT browser_extension_connect_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.app_users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.browser_extension_sessions
    ADD CONSTRAINT browser_extension_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.app_users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.connection_user_password_overrides
    ADD CONSTRAINT connection_user_password_overrides_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES public.resources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.connection_user_password_overrides
    ADD CONSTRAINT connection_user_password_overrides_password_resource_id_fkey FOREIGN KEY (password_resource_id) REFERENCES public.resources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.resource_secrets
    ADD CONSTRAINT resource_secrets_resource_id_fkey FOREIGN KEY (resource_id) REFERENCES public.resources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_invites
    ADD CONSTRAINT user_invites_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.app_users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_vault_unlocks
    ADD CONSTRAINT user_vault_unlocks_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.app_users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_vaults
    ADD CONSTRAINT user_vaults_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.app_users(id) ON DELETE CASCADE;
