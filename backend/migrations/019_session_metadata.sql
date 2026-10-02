-- Session revocation controls: sessions become listable and individually
-- revocable. Each row gets a public id (the token hash is never exposed),
-- the client that opened it (captured once at issue time), and — for web
-- sessions — a last-used timestamp (extension sessions already had one).
-- Rows are still hard-deleted on revoke; the audit log is the history.

alter table auth_sessions
    add column if not exists id uuid not null default gen_random_uuid();
alter table auth_sessions
    add column if not exists last_used_at timestamptz not null default now();
alter table auth_sessions
    add column if not exists created_ip text not null default '';
alter table auth_sessions
    add column if not exists user_agent text not null default '';
create unique index if not exists idx_auth_sessions_id on auth_sessions(id);

alter table browser_extension_sessions
    add column if not exists id uuid not null default gen_random_uuid();
alter table browser_extension_sessions
    add column if not exists created_ip text not null default '';
alter table browser_extension_sessions
    add column if not exists user_agent text not null default '';
create unique index if not exists idx_browser_extension_sessions_id
    on browser_extension_sessions(id);
