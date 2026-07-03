alter table resources add column if not exists folder_path text not null default '';
alter table resources add column if not exists launch_mode text not null default '';
alter table resources add column if not exists connection_domain text not null default '';
alter table resources add column if not exists connection_admin_session boolean not null default false;
alter table resources add column if not exists connection_automatic_logon boolean not null default false;
alter table resources add column if not exists connection_window_mode text not null default '';
alter table resources add column if not exists connection_use_multiple_monitors boolean not null default false;
alter table resources add column if not exists connection_show_connection_bar boolean not null default true;
alter table resources add column if not exists connection_screen_mode text not null default '';
alter table resources add column if not exists connection_mac_address text not null default '';

create index if not exists idx_resources_folder_path on resources(folder_path);
