create table if not exists local_groups (
    name text primary key,
    description text not null default '',
    rights text[] not null default '{}',
    mapped_external_groups text[] not null default '{}',
    assigned_user_ids text[] not null default '{}',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);
