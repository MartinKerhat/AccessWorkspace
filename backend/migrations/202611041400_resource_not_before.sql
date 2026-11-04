-- Key Vault secrets: the Azure `nbf` (not before / active from) attribute,
-- synced like expires_at mirrors `exp`. Informational in Azure; the catalog
-- shows a secret whose window has not started yet as `inactive`.
alter table resources add column if not exists not_before timestamptz;
