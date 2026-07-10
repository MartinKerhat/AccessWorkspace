-- Personal vaults: per-user asymmetric keypair for personal secrets.
-- public_key is stored in plaintext (saving encrypts TO it, no unlock needed);
-- the private key exists only wrapped by unlock methods or session tokens.

create table if not exists user_vaults (
    user_id text primary key references app_users(id) on delete cascade,
    public_key text not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

-- Unlock methods: each row is the SAME private key wrapped by a different
-- opener. method: 'password' (local login password, Argon2id-derived),
-- later 'passphrase' and 'passkey' (label carries the credential id).
create table if not exists user_vault_unlocks (
    user_id text not null references app_users(id) on delete cascade,
    method text not null,
    label text not null default '',
    wrapped_private_key text not null,
    salt text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    primary key (user_id, method, label)
);

-- Sessions carry the unlocked private key wrapped under the raw session
-- token (of which only the hash is stored) — a DB dump cannot unwrap these,
-- but any authenticated request can.
alter table auth_sessions
    add column if not exists vault_private_key text not null default '';
alter table browser_extension_sessions
    add column if not exists vault_private_key text not null default '';
alter table browser_extension_connect_tokens
    add column if not exists vault_private_key text not null default '';
