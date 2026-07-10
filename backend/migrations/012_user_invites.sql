-- One-time links for account setup: purpose 'invite' (new user sets their
-- first password — the admin never knows it) and 'reset' (admin-forced
-- password reset; the old vault and personal secrets were destroyed when the
-- link was issued). Tokens are stored hashed, like session tokens.

create table if not exists user_invites (
    token text primary key,
    user_id text not null references app_users(id) on delete cascade,
    purpose text not null default 'invite',
    created_by text not null default '',
    created_at timestamptz not null default now(),
    expires_at timestamptz not null
);

create index if not exists idx_user_invites_user_id on user_invites(user_id);
create index if not exists idx_user_invites_expires_at on user_invites(expires_at);
