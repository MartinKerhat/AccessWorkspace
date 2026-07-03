alter table resources add column if not exists personal boolean not null default false;
alter table resources add column if not exists owner_user_id text not null default '';

create index if not exists idx_resources_personal on resources(personal);
create index if not exists idx_resources_owner_user_id on resources(owner_user_id);

create table if not exists connection_user_password_overrides (
    connection_id text not null references resources(id) on delete cascade,
    user_id text not null,
    password_resource_id text not null references resources(id) on delete cascade,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    primary key (connection_id, user_id)
);

create index if not exists idx_connection_user_password_overrides_user_id
    on connection_user_password_overrides(user_id);
