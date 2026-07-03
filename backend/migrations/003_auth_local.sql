create table if not exists app_users (
    id text primary key,
    username text not null unique,
    display_name text not null,
    email text not null,
    password_hash text not null,
    groups text[] not null default '{}',
    is_admin boolean not null default false,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists auth_sessions (
    token text primary key,
    user_id text not null references app_users(id) on delete cascade,
    created_at timestamptz not null default now(),
    expires_at timestamptz not null
);

create index if not exists idx_auth_sessions_user_id on auth_sessions(user_id);
create index if not exists idx_auth_sessions_expires_at on auth_sessions(expires_at);

create table if not exists admin_settings (
    key text primary key,
    value text not null default '',
    updated_at timestamptz not null default now()
);
