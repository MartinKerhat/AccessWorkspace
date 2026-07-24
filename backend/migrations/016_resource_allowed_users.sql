-- Direct per-user sharing alongside group sharing. Stores app_users ids.
-- Visibility semantics: both allowed_groups and allowed_users empty = everyone
-- with category access; otherwise a match on either list grants access.
alter table resources add column if not exists allowed_users text[] not null default '{}';

create index if not exists idx_resources_allowed_users on resources using gin(allowed_users);
