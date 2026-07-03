alter table app_users
    add column if not exists workspace_blocked boolean not null default false;
