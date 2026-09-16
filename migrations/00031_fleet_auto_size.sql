-- +goose Up
-- The size of a run's fleet is decided by the launcher from the run's targets
-- unless a person overrides it. Record which, so a rerun of an auto-sized run
-- is auto-sized again rather than frozen at the number that came out last time.
ALTER TABLE run_fleet ADD COLUMN IF NOT EXISTS workers_auto boolean NOT NULL DEFAULT false;
-- 0 on a schedule means Auto. The old default of 2 was never a choice anyone
-- made in the dialog, so existing schedules at 2 become Auto too.
ALTER TABLE scan_schedule ALTER COLUMN worker_count SET DEFAULT 0;
UPDATE scan_schedule SET worker_count = 0 WHERE worker_count = 2;

-- +goose Down
UPDATE scan_schedule SET worker_count = 2 WHERE worker_count = 0;
ALTER TABLE scan_schedule ALTER COLUMN worker_count SET DEFAULT 2;
ALTER TABLE run_fleet DROP COLUMN IF EXISTS workers_auto;
