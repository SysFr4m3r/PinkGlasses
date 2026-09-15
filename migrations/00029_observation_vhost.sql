-- +goose Up
-- Web observations are per virtual host. Every web stage used to be addressed
-- by ip:port, so the request carried no SNI or Host header and every name on a
-- shared address was recorded as whatever the default server block returned —
-- a 403 from nginx where the browser shows the site. Targets now carry the
-- name, and what a name serves is stored under that name: host = '' is the
-- address itself (banner, version, address-only probes).
ALTER TABLE service_observation ADD COLUMN IF NOT EXISTS host text NOT NULL DEFAULT '';
ALTER TABLE service_observation DROP CONSTRAINT IF EXISTS service_observation_service_id_run_id_key;
ALTER TABLE service_observation ADD CONSTRAINT service_observation_service_id_run_id_host_key UNIQUE (service_id, run_id, host);
CREATE INDEX IF NOT EXISTS idx_obs_service_host ON service_observation (service_id, host);

-- +goose Down
DROP INDEX IF EXISTS idx_obs_service_host;
ALTER TABLE service_observation DROP CONSTRAINT IF EXISTS service_observation_service_id_run_id_host_key;
DELETE FROM service_observation WHERE host <> '';
ALTER TABLE service_observation ADD CONSTRAINT service_observation_service_id_run_id_key UNIQUE (service_id, run_id);
ALTER TABLE service_observation DROP COLUMN IF EXISTS host;
