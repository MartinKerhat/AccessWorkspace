create table if not exists app_registration_notification_policies (
    resource_id text not null references resources(id) on delete cascade,
    credential_key_id text not null default '',
    enabled boolean not null,
    reminder_days integer[] not null default '{}',
    channels text[] not null default '{}',
    updated_at timestamptz not null default now(),
    primary key (resource_id, credential_key_id)
);

create table if not exists app_registration_notifications (
    id text primary key,
    user_id text not null references app_users(id) on delete cascade,
    resource_id text not null references resources(id) on delete cascade,
    resource_name text not null default '',
    credential_key_id text not null,
    credential_display_name text not null default '',
    credential_type text not null default '',
    credential_end_date_time timestamptz null,
    reminder_day integer not null,
    title text not null,
    body text not null,
    channels text[] not null default '{}',
    read_at timestamptz null,
    email_status text not null default '',
    email_sent_at timestamptz null,
    email_error text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (user_id, resource_id, credential_key_id, credential_end_date_time, reminder_day)
);

create index if not exists idx_app_registration_notifications_user_id on app_registration_notifications(user_id);
create index if not exists idx_app_registration_notifications_read_at on app_registration_notifications(read_at);
create index if not exists idx_app_registration_notifications_created_at on app_registration_notifications(created_at desc);
