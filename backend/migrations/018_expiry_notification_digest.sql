-- Expiry reminder emails are now batched: the sync sweep records a reminder row
-- per object and one digest per recipient goes out at the end of the run. The
-- flush that assembles those digests runs on every sync tick and asks the same
-- question each time — "which rows are still owed an email?" — so it gets a
-- partial index rather than a sequential scan over the whole reminder history.
create index if not exists idx_expiry_notifications_pending_email
    on expiry_notifications (user_id, reminder_day)
    where read_at is null and email_status <> 'sent';
