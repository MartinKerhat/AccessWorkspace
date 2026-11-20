-- The notification table (still named expiry_notifications: renaming it is
-- churn with no user value) now holds two kinds of rows:
--   'expiry' — credential / secret expiry reminders (every existing row);
--   'share'  — "X shared Y with you", written when a person saves an object
--              and someone gains access to it. Share rows leave the credential
--              columns empty and are never superseded or pulled into the
--              expiry email digest.
alter table expiry_notifications add column if not exists kind text not null default 'expiry';
