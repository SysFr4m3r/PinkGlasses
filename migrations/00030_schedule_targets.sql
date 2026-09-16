-- +goose Up
-- A schedule remembers which targets it covers. Empty means every non-excluded
-- target at the time each run starts, which is what every schedule did before.
ALTER TABLE scan_schedule ADD COLUMN IF NOT EXISTS targets text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE scan_schedule DROP COLUMN IF EXISTS targets;
