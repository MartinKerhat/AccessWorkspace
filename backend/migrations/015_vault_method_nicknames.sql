-- Human names for vault unlock methods. Only passkeys need one (a user can
-- hold several — one per PC/phone — and credential IDs are opaque, so without
-- a name "remove the old laptop's passkey" is a guessing game). Passphrase
-- and login-password rows are unique per vault and get fixed display names
-- in the UI instead.
alter table user_vault_unlocks
    add column if not exists nickname text not null default '';
