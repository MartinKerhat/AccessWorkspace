create table if not exists resources (
    id text primary key,
    name text not null,
    type text not null,
    description text not null default '',
    owner text not null default '',
    target_host text not null default '',
    target_port integer null,
    username text not null default '',
    launch_allowed boolean not null default false,
    reveal_allowed boolean not null default false,
    allowed_groups text[] not null default '{}',
    tags text[] not null default '{}',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    archived_at timestamptz null
);

create table if not exists resource_secrets (
    resource_id text primary key references resources(id) on delete cascade,
    secret_mode text not null,
    secret_value text not null default '',
    secret_reference text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists audit_events (
    id text primary key,
    event_type text not null,
    user_id text not null,
    user_name text not null,
    resource_id text null,
    resource_name text null,
    metadata jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now()
);

create index if not exists idx_resources_type on resources(type);
create index if not exists idx_resources_archived_at on resources(archived_at);
create index if not exists idx_resources_allowed_groups on resources using gin(allowed_groups);
create index if not exists idx_resources_tags on resources using gin(tags);
create index if not exists idx_audit_events_user_id on audit_events(user_id);
create index if not exists idx_audit_events_resource_id on audit_events(resource_id);
