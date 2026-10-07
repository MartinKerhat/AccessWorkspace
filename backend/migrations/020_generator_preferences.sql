-- Generator page: each user's last-used settings per generator part
-- (password length and classes, passphrase words, key size/encoding, …),
-- so the web UI and later the browser extension open with what that person
-- actually uses instead of a global default. One JSON document per part;
-- rows vanish with the user.
create table if not exists user_generator_preferences (
    user_id text not null references app_users(id) on delete cascade,
    part text not null,
    settings jsonb not null default '{}'::jsonb,
    updated_at timestamptz not null default now(),
    primary key (user_id, part)
);
