-- At most one active catalog object per Azure Key Vault secret. Before this,
-- auto-import re-imported a secret whose object a user had archived, and
-- restoring the old object then left two active objects for one secret.
--
-- Existing collisions keep the oldest object; the others are archived (not
-- deleted), with an audit event saying why, so they show up in Archived.
with ranked as (
    select id, name,
        row_number() over (partition by rtrim(source_object_id, '/') order by created_at, id) as position
    from resources
    where archived_at is null
        and type = 'key_vault_secret'
        and source_kind = 'azure_key_vault'
        and source_object_id <> ''
), archived as (
    update resources r
    set archived_at = now(), updated_at = now()
    from ranked
    where ranked.id = r.id
        and ranked.position > 1
    returning r.id, r.name
)
insert into audit_events (id, event_type, user_id, user_name, resource_id, resource_name, metadata)
select gen_random_uuid()::text, 'resource_archived', 'system:migration', 'System migration', id, name,
    jsonb_build_object('type', 'key_vault_secret', 'reason', 'duplicate_key_vault_object')
from archived;

create unique index if not exists idx_resources_active_keyvault_secret
    on resources (rtrim(source_object_id, '/'))
    where archived_at is null
        and type = 'key_vault_secret'
        and source_kind = 'azure_key_vault'
        and source_object_id <> '';
