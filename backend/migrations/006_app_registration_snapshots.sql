create table if not exists app_registration_credentials (
    resource_id text not null references resources(id) on delete cascade,
    key_id text not null,
    credential_type text not null,
    display_name text not null default '',
    start_date_time timestamptz null,
    end_date_time timestamptz null,
    hint text not null default '',
    usage text not null default '',
    last_synced_at timestamptz null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    primary key (resource_id, key_id, credential_type)
);

create index if not exists idx_app_registration_credentials_resource_id on app_registration_credentials(resource_id);
create index if not exists idx_app_registration_credentials_end_date_time on app_registration_credentials(end_date_time);

create table if not exists app_registration_owners (
    resource_id text not null references resources(id) on delete cascade,
    owner_id text not null,
    owner_type text not null default '',
    display_name text not null default '',
    email text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    primary key (resource_id, owner_id)
);

create index if not exists idx_app_registration_owners_resource_id on app_registration_owners(resource_id);

insert into app_registration_credentials (
    resource_id, key_id, credential_type, display_name, end_date_time, last_synced_at
)
select
    id,
    coalesce(nullif(source_object_id, ''), id) || ':summary',
    credential_type,
    'Imported credential summary',
    credential_expires_at,
    last_synced_at
from resources
where type = 'app_registration'
    and credential_type <> ''
    and credential_expires_at is not null
on conflict do nothing;
