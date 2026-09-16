package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/benlik386/pinkglasses/internal/domain"
)

// TargetGroup is a named list of targets: what one "Add targets" produced.
// It is the unit the Dashboard shows and the scan dialog picks.
type TargetGroup struct {
	ID        uuid.UUID            `json:"id"`
	ScopeID   uuid.UUID            `json:"scope_id"`
	Name      string               `json:"name"`
	CreatedAt time.Time            `json:"created_at"`
	Targets   []domain.ScopeTarget `json:"targets"`
	// Authorized: every entry is active with a recorded authorization, so the
	// group as a whole may be scanned actively.
	Authorized bool `json:"authorized"`
}

// ErrGroupExists is returned when a group would take a name another group in
// the company already has.
var ErrGroupExists = errors.New("a group with that name already exists in this company")

func groupAuthorized(ts []domain.ScopeTarget) bool {
	if len(ts) == 0 {
		return false
	}
	for _, t := range ts {
		if !t.Authorized() {
			return false
		}
	}
	return true
}

// ListTargetGroups returns a company's groups with their entries, oldest first.
func (s *Store) ListTargetGroups(ctx context.Context, scopeID uuid.UUID) ([]TargetGroup, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, scope_id, name, created_at FROM target_group WHERE scope_id=$1 ORDER BY created_at, name`, scopeID)
	if err != nil {
		return nil, err
	}
	groups := []TargetGroup{}
	index := map[uuid.UUID]int{}
	for rows.Next() {
		var g TargetGroup
		if err := rows.Scan(&g.ID, &g.ScopeID, &g.Name, &g.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		g.Targets = []domain.ScopeTarget{}
		index[g.ID] = len(groups)
		groups = append(groups, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	targets, err := s.ListTargets(ctx, scopeID, "")
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		if t.GroupID == nil {
			continue
		}
		if i, ok := index[*t.GroupID]; ok {
			groups[i].Targets = append(groups[i].Targets, t)
		}
	}
	for i := range groups {
		groups[i].Authorized = groupAuthorized(groups[i].Targets)
	}
	return groups, nil
}

// GetTargetGroup returns one group of a company with its entries.
func (s *Store) GetTargetGroup(ctx context.Context, scopeID, id uuid.UUID) (TargetGroup, bool, error) {
	var g TargetGroup
	err := s.Pool.QueryRow(ctx, `SELECT id, scope_id, name, created_at FROM target_group WHERE id=$1 AND scope_id=$2`, id, scopeID).
		Scan(&g.ID, &g.ScopeID, &g.Name, &g.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, false, nil
	}
	if err != nil {
		return g, false, err
	}
	g.Targets = []domain.ScopeTarget{}
	all, err := s.ListTargets(ctx, scopeID, "")
	if err != nil {
		return g, false, err
	}
	for _, t := range all {
		if t.GroupID != nil && *t.GroupID == id {
			g.Targets = append(g.Targets, t)
		}
	}
	g.Authorized = groupAuthorized(g.Targets)
	return g, true, nil
}

// CreateTargetGroup adds an empty group.
func (s *Store) CreateTargetGroup(ctx context.Context, scopeID uuid.UUID, name string) (TargetGroup, error) {
	var g TargetGroup
	err := s.Pool.QueryRow(ctx, `INSERT INTO target_group (scope_id, name) VALUES ($1,$2) RETURNING id, scope_id, name, created_at`, scopeID, name).
		Scan(&g.ID, &g.ScopeID, &g.Name, &g.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return g, ErrGroupExists
	}
	g.Targets = []domain.ScopeTarget{}
	return g, err
}

// EnsureTargetGroup returns the company's group with that name, creating it.
func (s *Store) EnsureTargetGroup(ctx context.Context, scopeID uuid.UUID, name string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO target_group (scope_id, name) VALUES ($1,$2)
		ON CONFLICT (scope_id, name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, scopeID, name).Scan(&id)
	return id, err
}

// RenameTargetGroup changes a group's name.
func (s *Store) RenameTargetGroup(ctx context.Context, scopeID, id uuid.UUID, name string) (bool, error) {
	ct, err := s.Pool.Exec(ctx, `UPDATE target_group SET name=$3 WHERE id=$1 AND scope_id=$2`, id, scopeID, name)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return false, ErrGroupExists
	}
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// DeleteTargetGroup removes a group and, through the foreign key, its entries.
func (s *Store) DeleteTargetGroup(ctx context.Context, scopeID, id uuid.UUID) (bool, error) {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM target_group WHERE id=$1 AND scope_id=$2`, id, scopeID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// DeleteGroupTargetsNotIn removes the group's entries whose value is not in
// keep — what editing the list down to fewer lines means.
func (s *Store) DeleteGroupTargetsNotIn(ctx context.Context, groupID uuid.UUID, keep []string) (int64, error) {
	if keep == nil {
		keep = []string{}
	}
	ct, err := s.Pool.Exec(ctx, `DELETE FROM scope_target WHERE group_id=$1 AND NOT (value = ANY($2))`, groupID, keep)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

// SetGroupTargets applies mode, tags and authorization to every entry of a
// group: the group is edited as one thing.
func (s *Store) SetGroupTargets(ctx context.Context, groupID uuid.UUID, mode domain.TargetMode, tags []string, authBy *string, authAt *time.Time) error {
	if tags == nil {
		tags = []string{}
	}
	_, err := s.Pool.Exec(ctx, `UPDATE scope_target SET mode=$2, tags=$3, authorized_by=$4, authorized_at=$5 WHERE group_id=$1`,
		groupID, mode, tags, authBy, authAt)
	return err
}

// GroupValues returns the target values of the given groups in a company —
// what a run over those groups covers.
func (s *Store) GroupValues(ctx context.Context, scopeID uuid.UUID, ids []uuid.UUID) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx, `SELECT value FROM scope_target WHERE scope_id=$1 AND group_id = ANY($2) ORDER BY value`, scopeID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
