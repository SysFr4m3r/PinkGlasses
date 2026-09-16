-- +goose Up
-- Targets come in named groups: what a person pastes into "Add targets" is one
-- entry with a name and a list of domains, IPs and CIDRs. The scan dialog picks
-- groups and a schedule remembers groups, so editing a group later changes what
-- its scheduled runs cover. Every existing target becomes a group of one named
-- after its value, so nothing collapses unexpectedly.
CREATE TABLE IF NOT EXISTS target_group (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scope_id   uuid NOT NULL REFERENCES scope(id) ON DELETE CASCADE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (scope_id, name)
);
ALTER TABLE scope_target ADD COLUMN IF NOT EXISTS group_id uuid REFERENCES target_group(id) ON DELETE CASCADE;
INSERT INTO target_group (scope_id, name)
  SELECT DISTINCT scope_id, value FROM scope_target WHERE group_id IS NULL
  ON CONFLICT DO NOTHING;
UPDATE scope_target t SET group_id = g.id
  FROM target_group g WHERE t.group_id IS NULL AND g.scope_id = t.scope_id AND g.name = t.value;
CREATE INDEX IF NOT EXISTS idx_scope_target_group ON scope_target (group_id);
ALTER TABLE scan_schedule ADD COLUMN IF NOT EXISTS target_group_ids uuid[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE scan_schedule DROP COLUMN IF EXISTS target_group_ids;
ALTER TABLE scope_target DROP COLUMN IF EXISTS group_id;
DROP TABLE IF EXISTS target_group;
