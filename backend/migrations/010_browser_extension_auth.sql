create table if not exists browser_extension_connect_tokens (
    token text primary key,
    user_id text not null references app_users(id) on delete cascade,
    auth_mode text not null,
    created_at timestamptz not null default now(),
    expires_at timestamptz not null
);

create index if not exists idx_browser_extension_connect_tokens_user_id
    on browser_extension_connect_tokens(user_id);

create index if not exists idx_browser_extension_connect_tokens_expires_at
    on browser_extension_connect_tokens(expires_at);

create table if not exists browser_extension_sessions (
    token text primary key,
    user_id text not null references app_users(id) on delete cascade,
    installation_id text not null,
    created_at timestamptz not null default now(),
    last_used_at timestamptz not null default now(),
    expires_at timestamptz not null
);

create unique index if not exists idx_browser_extension_sessions_installation_id
    on browser_extension_sessions(installation_id);

create index if not exists idx_browser_extension_sessions_user_id
    on browser_extension_sessions(user_id);

create index if not exists idx_browser_extension_sessions_expires_at
    on browser_extension_sessions(expires_at);
