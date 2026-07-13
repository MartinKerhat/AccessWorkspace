-- Brute-force lockout for local login. Counters live on the account so the
-- lock holds across all replicas (unlike the in-memory per-IP throttle).

alter table app_users
    add column if not exists failed_login_attempts integer not null default 0;
alter table app_users
    add column if not exists locked_until timestamptz null;
