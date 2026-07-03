package seed

import (
	"context"

	"access-workspace/backend/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Run(ctx context.Context, pool *pgxpool.Pool) error {
	var userCount int
	if err := pool.QueryRow(ctx, `select count(*) from app_users`).Scan(&userCount); err != nil {
		return err
	}
	if userCount == 0 {
		if err := seedUsers(ctx, pool); err != nil {
			return err
		}
	}

	var groupCount int
	if err := pool.QueryRow(ctx, `select count(*) from local_groups`).Scan(&groupCount); err != nil {
		return err
	}
	if groupCount == 0 {
		if err := seedLocalGroups(ctx, pool); err != nil {
			return err
		}
	}

	var count int
	if err := pool.QueryRow(ctx, `select count(*) from resources`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	_, err := pool.Exec(ctx, `
		insert into resources (
			id, name, type, description, owner, owner_team, environment, status, source_kind, source_object_id,
			notes, target_host, target_port, target_url, target_system, username,
			vault_name, object_name, object_type, provider, application_id, tenant_id, credential_type, credential_expires_at,
			linked_secret_ref, launch_allowed, reveal_allowed, copy_allowed, allowed_groups
		) values
			('res-bastion', 'Platform Bastion', 'ssh', 'Shared SSH bastion for platform maintenance', 'Platform Team', 'Platform', 'prod', 'active', 'manual', '', 'Primary bastion for platform operations', 'bastion.internal', 22, '', '', 'platform-admin', '', '', '', '', '', '', '', null, '', true, true, true, '{"platform","ops-admins"}'),
			('res-rdp', 'Finance Jump Host', 'rdp', 'RDP access point for finance reporting workloads', 'Finance Ops', 'Finance', 'prod', 'active', 'manual', '', 'Used for reporting support and month-end operations', 'fin-jump.internal', 3389, '', '', 'finops-user', '', '', '', '', '', '', '', null, '', true, false, false, '{"support","ops-admins"}'),
			('res-web', 'Kibana Prod', 'web_portal', 'Production observability portal', 'SRE', 'Observability', 'prod', 'active', 'manual', '', 'Shared portal access for operational troubleshooting', '', null, 'https://kibana.internal.example', 'Kibana', '', '', '', '', '', '', '', '', null, '', true, false, false, '{"support","platform","network"}'),
			('res-secret', 'Legacy Billing Credential', 'shared_secret', 'Shared credential for a legacy billing workflow', 'Billing Ops', 'Billing', 'prod', 'active', 'manual', '', 'Credential used during legacy vendor reconciliations', '', null, 'https://billing.internal.example', 'Billing portal', 'billing-shared', '', '', '', '', '', '', '', null, '', false, true, true, '{"support"}'),
			('res-kv', 'Payroll KV Secret', 'key_vault_secret', 'Payroll secret metadata from Azure Key Vault', 'HR Systems', 'HR Platforms', 'prod', 'active', 'azure_key_vault', 'https://payroll-vault.vault.azure.net/secrets/payroll-api-password', 'External metadata record; secret value stays in Key Vault', '', null, '', '', '', 'payroll-vault', 'payroll-api-password', 'secret', '', '', '', '', '2026-12-31T00:00:00Z', '', false, true, true, '{"ops-admins"}'),
			('res-appreg', 'Grafana App Registration', 'app_registration', 'Operational view of the Grafana Entra application registration', 'Identity Team', 'Identity', 'prod', 'active', 'entra_app_registration', 'grafana-app-reg', 'External application metadata with local ownership overlay', '', null, '', '', '', '', '', '', 'entra', 'grafana-app-reg', 'core-tenant', 'client_secret', '2026-09-01T00:00:00Z', 'azure-key-vault://shared/appregs/grafana-prod', false, false, false, '{"platform"}')
	`)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		insert into resource_secrets (resource_id, secret_mode, secret_value, secret_reference) values
			('res-bastion', 'inline', 'Sup3rSshSecret!', ''),
			('res-rdp', 'external_reference', '', 'secret://finance/jump-host/password'),
			('res-web', 'external_reference', '', 'url://kibana.internal.example'),
			('res-secret', 'inline', 'Billing-Password-2026', ''),
			('res-kv', 'external_reference', '', 'azure-key-vault://payroll/ops-password'),
			('res-appreg', 'external_reference', '', 'app-registration://grafana-prod')
	`)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		insert into app_registration_credentials (
			resource_id, key_id, credential_type, display_name, start_date_time, end_date_time, hint, usage
		) values
			('res-appreg', 'grafana-secret-2026', 'client_secret', 'Grafana production secret', '2026-01-01T00:00:00Z', '2026-09-01T00:00:00Z', 'prod', ''),
			('res-appreg', 'grafana-cert-2027', 'certificate', 'Grafana workload certificate', '2026-01-01T00:00:00Z', '2027-03-01T00:00:00Z', '', 'Verify')
		on conflict do nothing
	`)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		insert into app_registration_owners (
			resource_id, owner_id, owner_type, display_name, email
		) values
			('res-appreg', 'alice', 'user', 'Alice Admin', 'alice@example.internal')
		on conflict do nothing
	`)
	return err
}

func seedUsers(ctx context.Context, pool *pgxpool.Pool) error {
	passwordHash, err := auth.HashPassword("123456")
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		insert into app_users (id, username, display_name, email, password_hash, groups, is_admin) values
			('alice', 'alice', 'Alice Admin', 'alice@example.internal', $1, '{"ops-admins","platform","engineering"}', true),
			('sam', 'sam', 'Sam Support', 'sam@example.internal', $1, '{"support","engineering"}', false),
			('nina', 'nina', 'Nina Network', 'nina@example.internal', $1, '{"network","platform"}', false),
			('wendy', 'wendy', 'Wendy Web', 'wendy@example.internal', $1, '{"web","support"}', false)
	`, passwordHash)
	return err
}

func seedLocalGroups(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		insert into local_groups (name, description, rights, mapped_external_groups, assigned_user_ids) values
			('ops-admins', 'Workspace administrators with full operational access.', '{"connections.read","connections.edit","keyvault.read","keyvault.edit","appregistrations.read","appregistrations.edit","passwords.read","passwords.edit","audit.read","admin.access"}', '{}', '{"alice"}'),
			('platform', 'Platform operators for shared infrastructure access.', '{"connections.read","keyvault.read","appregistrations.read","passwords.read"}', '{}', '{}'),
			('engineering', 'Engineering teams with app registration and shared password visibility.', '{"connections.read","appregistrations.read","passwords.read"}', '{}', '{}'),
			('support', 'Support users for shared passwords and supported connections.', '{"connections.read","passwords.read"}', '{}', '{}'),
			('network', 'Network operations users for infrastructure and portal access.', '{"connections.read","passwords.read"}', '{}', '{}'),
			('web', 'Web and portal operators for shared password access.', '{"passwords.read"}', '{}', '{}')
	`)
	return err
}
