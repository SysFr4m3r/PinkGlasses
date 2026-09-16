-- +goose Up
-- A value may sit in several groups of one company. The old rule allowed it
-- once per company, so adding it to a second group moved it out of the first.
-- Uniqueness is now per group; every row has a group since 00032.
ALTER TABLE scope_target ALTER COLUMN group_id SET NOT NULL;
ALTER TABLE scope_target DROP CONSTRAINT IF EXISTS scope_target_scope_id_kind_value_key;
ALTER TABLE scope_target ADD CONSTRAINT scope_target_group_id_kind_value_key UNIQUE (group_id, kind, value);

-- +goose Down
-- Keep one row per (scope, kind, value) — the oldest — before restoring the old rule.
DELETE FROM scope_target t USING scope_target o
  WHERE t.scope_id = o.scope_id AND t.kind = o.kind AND t.value = o.value AND t.created_at > o.created_at;
ALTER TABLE scope_target DROP CONSTRAINT IF EXISTS scope_target_group_id_kind_value_key;
ALTER TABLE scope_target ADD CONSTRAINT scope_target_scope_id_kind_value_key UNIQUE (scope_id, kind, value);
ALTER TABLE scope_target ALTER COLUMN group_id DROP NOT NULL;
