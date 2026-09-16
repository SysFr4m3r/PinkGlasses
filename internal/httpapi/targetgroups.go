package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/benlik386/pinkglasses/internal/domain"
	"github.com/benlik386/pinkglasses/internal/store"
)

// Target groups: what one "Add targets" produces — a name and its list of
// domains, IPs and CIDRs — edited, removed and scanned as one thing.

type groupInput struct {
	Name      *string  `json:"name"`
	Values    []string `json:"values"`
	Tags      []string `json:"tags"`
	Authorize *bool    `json:"authorize"`
}

func cleanValues(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range raw {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func (s *Server) listTargetGroups(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuid.Parse(chi.URLParam(r, "scopeID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad scope id")
		return
	}
	groups, err := s.st.ListTargetGroups(r.Context(), scopeID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

// createTargetGroup makes a group from a name and a list of values. The name
// defaults to the first value.
func (s *Server) createTargetGroup(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuid.Parse(chi.URLParam(r, "scopeID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad scope id")
		return
	}
	var in groupInput
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad body")
		return
	}
	values := cleanValues(in.Values)
	if len(values) == 0 {
		writeErr(w, http.StatusBadRequest, "values required: one domain, IP or CIDR per line")
		return
	}
	name := values[0]
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		name = strings.TrimSpace(*in.Name)
	}
	g, err := s.st.CreateTargetGroup(r.Context(), scopeID, name)
	if errors.Is(err, store.ErrGroupExists) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	authorize := in.Authorize != nil && *in.Authorize
	if _, err := s.addTargetValues(r, scopeID, &g.ID, values, in.Tags, authorize); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	full, _, err := s.st.GetTargetGroup(r.Context(), scopeID, g.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.auditReq(r, "target_group.create", g.ID.String(), map[string]any{"name": name, "entries": len(values), "authorized": authorize})
	writeJSON(w, http.StatusCreated, full)
}

// patchTargetGroup edits a group as one thing: its name, its list (entries
// added, entries not in the list removed), and the tags and authorization of
// every entry.
func (s *Server) patchTargetGroup(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuid.Parse(chi.URLParam(r, "scopeID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad scope id")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "groupID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad group id")
		return
	}
	cur, ok, err := s.st.GetTargetGroup(r.Context(), scopeID, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "group not found in this company")
		return
	}
	var in groupInput
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad body")
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" && strings.TrimSpace(*in.Name) != cur.Name {
		if _, err := s.st.RenameTargetGroup(r.Context(), scopeID, id, strings.TrimSpace(*in.Name)); err != nil {
			if errors.Is(err, store.ErrGroupExists) {
				writeErr(w, http.StatusConflict, err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	// What the entries should look like after the edit: the given tags and
	// authorization, or the group's current ones.
	tags := in.Tags
	if tags == nil {
		if len(cur.Targets) > 0 {
			tags = cur.Targets[0].Tags
		} else {
			tags = []string{}
		}
	}
	authorize := cur.Authorized
	if in.Authorize != nil {
		authorize = *in.Authorize
	}
	mode := domain.ModePassiveOnly
	var authBy *string
	var authAt *time.Time
	if authorize {
		mode = domain.ModeActive
		if cur.Authorized && in.Authorize == nil {
			// Unchanged: keep the existing record rather than re-stamping it.
			authBy, authAt = cur.Targets[0].AuthorizedBy, cur.Targets[0].AuthorizedAt
		} else {
			a := actor(r)
			now := time.Now()
			authBy, authAt = &a, &now
		}
	}
	removed := int64(0)
	if in.Values != nil {
		values := cleanValues(in.Values)
		if len(values) == 0 {
			writeErr(w, http.StatusBadRequest, "a group needs at least one entry; remove the group instead")
			return
		}
		if removed, err = s.st.DeleteGroupTargetsNotIn(r.Context(), id, values); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := s.addTargetValues(r, scopeID, &id, values, tags, authorize); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := s.st.SetGroupTargets(r.Context(), id, mode, tags, authBy, authAt); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	full, _, err := s.st.GetTargetGroup(r.Context(), scopeID, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.auditReq(r, "target_group.update", id.String(), map[string]any{
		"name": full.Name, "entries": len(full.Targets), "removed": removed, "authorized": full.Authorized})
	writeJSON(w, http.StatusOK, full)
}

func (s *Server) deleteTargetGroup(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuid.Parse(chi.URLParam(r, "scopeID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad scope id")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "groupID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad group id")
		return
	}
	ok, err := s.st.DeleteTargetGroup(r.Context(), scopeID, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "group not found in this company")
		return
	}
	s.auditReq(r, "target_group.delete", id.String(), map[string]any{"scope_id": scopeID.String()})
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// addTargetValues inserts values into a company, optionally into a group,
// with the mode the authorization tick implies. Shared by the group handlers
// and the plain targets endpoint.
func (s *Server) addTargetValues(r *http.Request, scopeID uuid.UUID, groupID *uuid.UUID, values, tags []string, authorize bool) ([]domain.ScopeTarget, error) {
	mode := domain.ModePassiveOnly
	var authBy *string
	var authAt *time.Time
	if authorize {
		mode = domain.ModeActive
		a := actor(r)
		now := time.Now()
		authBy, authAt = &a, &now
	}
	if tags == nil {
		tags = []string{}
	}
	var out []domain.ScopeTarget
	for _, v := range values {
		gid := groupID
		if gid == nil {
			// No group given: the value is its own group, named after itself,
			// the way every pre-existing target was migrated.
			id, err := s.st.EnsureTargetGroup(r.Context(), scopeID, v)
			if err != nil {
				return out, err
			}
			gid = &id
		}
		t := domain.ScopeTarget{
			ScopeID: scopeID, Kind: guessKind(v), Value: v, Tags: tags, Mode: mode,
			AuthorizedBy: authBy, AuthorizedAt: authAt, GroupID: gid,
		}
		saved, err := s.st.AddTarget(r.Context(), t)
		if err != nil {
			return out, err
		}
		out = append(out, saved)
	}
	return out, nil
}
