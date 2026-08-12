-- Expiry reminders stopped being app-registration-only when Key Vault secrets
-- joined them (they carry an expiry too), so the app_registration_* names
-- became misleading. The columns still fit both sources: credential_key_id is
-- the credential key id for an app registration and the secret name for a Key
-- Vault secret, credential_type is "secret"/"certificate" either way.
alter table if exists app_registration_notifications rename to expiry_notifications;
alter table if exists app_registration_notification_policies rename to expiry_notification_policies;

-- Renaming a table leaves its index names behind, so rename those too rather
-- than keeping idx_app_registration_* on a table that is no longer called that.
alter index if exists idx_app_registration_notifications_user_id rename to idx_expiry_notifications_user_id;
alter index if exists idx_app_registration_notifications_read_at rename to idx_expiry_notifications_read_at;
alter index if exists idx_app_registration_notifications_created_at rename to idx_expiry_notifications_created_at;
