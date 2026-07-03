alter table resources add column if not exists owner_team text not null default '';
alter table resources add column if not exists environment text not null default '';
alter table resources add column if not exists status text not null default 'active';
alter table resources add column if not exists source_kind text not null default 'manual';
alter table resources add column if not exists source_object_id text not null default '';
alter table resources add column if not exists last_synced_at timestamptz null;
alter table resources add column if not exists notes text not null default '';
alter table resources add column if not exists target_url text not null default '';
alter table resources add column if not exists target_system text not null default '';
alter table resources add column if not exists vault_name text not null default '';
alter table resources add column if not exists object_name text not null default '';
alter table resources add column if not exists object_type text not null default '';
alter table resources add column if not exists object_version text not null default '';
alter table resources add column if not exists content_type text not null default '';
alter table resources add column if not exists expires_at timestamptz null;
alter table resources add column if not exists provider text not null default '';
alter table resources add column if not exists application_id text not null default '';
alter table resources add column if not exists tenant_id text not null default '';
alter table resources add column if not exists client_id text not null default '';
alter table resources add column if not exists credential_type text not null default '';
alter table resources add column if not exists credential_expires_at timestamptz null;
alter table resources add column if not exists display_name_external text not null default '';
alter table resources add column if not exists linked_secret_ref text not null default '';
alter table resources add column if not exists copy_allowed boolean not null default false;

update resources
set
    target_url = case when type = 'web_portal' then target_host else target_url end,
    target_system = case when type = 'shared_secret' then target_host else target_system end,
    vault_name = case when type = 'key_vault_secret' then target_host else vault_name end,
    object_name = case when type = 'key_vault_secret' then name else object_name end,
    object_type = case when type = 'key_vault_secret' then 'secret' else object_type end,
    provider = case when type = 'app_registration' then 'entra' else provider end,
    application_id = case when type = 'app_registration' then target_host else application_id end,
    copy_allowed = reveal_allowed;

drop index if exists idx_resources_tags;
alter table resources drop column if exists tags;
