alter table app_users
    add column if not exists direct_rights text[] not null default '{}';
