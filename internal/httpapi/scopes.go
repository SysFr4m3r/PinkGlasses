package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/benlik386/pinkglasses/internal/domain"
)

func (s *Server) createScope(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil || in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	sc, err := s.st.CreateScope(r.Context(), in.Name, actor(r), userIDOf(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.auditReq(r, "scope.create", sc.ID.String(), map[string]any{"name": sc.Name})
	writeJSON(w, http.StatusCreated, sc)
}

// listScopes returns every company, or only the caller's own with ?mine=true.
//
// "Own" means created by the same actor, which today is whatever
// X-Forwarded-User says or "local" — so this narrows a shared list, it does not
// protect anything. Until Phase 17 verifies identity, a caller can see any
// company by simply not asking for the filter.
func (s *Server) listScopes(w http.ResponseWriter, r *http.Request) {
	owner := ""
	if r.URL.Query().Get("mine") == "true" {
		owner = actor(r)
	}
	list, err := s.st.ListScopes(r.Context(), owner)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) scopeSummary(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "scopeID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad scope id")
		return
	}
	sum, err := s.st.Summary(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

// patchTarget edits a target's mode, tags and authorization. The value is the
// target's identity and is not editable here. Authorization is explicit: a
// switch to active carries authorize:true to be recorded as authorized; any
// other mode clears the record, since it no longer means anything.
func (s *Server) patchTarget(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuid.Parse(chi.URLParam(r, "scopeID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad scope id")
		return
	}
	targetID, err := uuid.Parse(chi.URLParam(r, "targetID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad target id")
		return
	}
	var in struct {
		Mode      *string  `json:"mode"`
		Tags      []string `json:"tags"`
		Authorize *bool    `json:"authorize"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad body")
		return
	}
	cur, ok, err := s.st.GetTarget(r.Context(), scopeID, targetID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "target not found in this company")
		return
	}
	mode := cur.Mode
	if in.Mode != nil {
		switch domain.TargetMode(*in.Mode) {
		case domain.ModeActive, domain.ModePassiveOnly, domain.ModeExclude:
			mode = domain.TargetMode(*in.Mode)
		default:
			writeErr(w, http.StatusBadRequest, "mode must be active, passive_only or exclude")
			return
		}
	}
	tags := cur.Tags
	if in.Tags != nil {
		tags = in.Tags
	}
	authBy, authAt := cur.AuthorizedBy, cur.AuthorizedAt
	switch {
	case mode != domain.ModeActive:
		authBy, authAt = nil, nil
	case in.Authorize != nil && *in.Authorize:
		a := actor(r)
		now := time.Now()
		authBy, authAt = &a, &now
	case in.Authorize != nil && !*in.Authorize:
		authBy, authAt = nil, nil
	}
	t, ok, err := s.st.UpdateTarget(r.Context(), scopeID, targetID, mode, tags, authBy, authAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "target not found in this company")
		return
	}
	s.auditReq(r, "target.update", targetID.String(), map[string]any{
		"value": t.Value, "mode": string(t.Mode), "authorized": t.Authorized(), "tags": t.Tags})
	writeJSON(w, http.StatusOK, t)
}

// deleteTarget takes a target out of a company. Future runs no longer cover it;
// what earlier runs found under it stays in the inventory.
func (s *Server) deleteTarget(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuid.Parse(chi.URLParam(r, "scopeID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad scope id")
		return
	}
	targetID, err := uuid.Parse(chi.URLParam(r, "targetID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad target id")
		return
	}
	ok, err := s.st.DeleteTarget(r.Context(), scopeID, targetID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "target not found in this company")
		return
	}
	s.auditReq(r, "target.delete", targetID.String(), map[string]any{"scope_id": scopeID.String()})
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) addTarget(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuid.Parse(chi.URLParam(r, "scopeID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad scope id")
		return
	}
	var in struct {
		Kind      string   `json:"kind"`
		Value     string   `json:"value"`
		Values    []string `json:"values"` // bulk import
		Tags      []string `json:"tags"`
		Mode      string   `json:"mode"`
		Authorize bool     `json:"authorize"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad body")
		return
	}
	values := in.Values
	if in.Value != "" {
		values = append(values, in.Value)
	}
	if len(values) == 0 {
		writeErr(w, http.StatusBadRequest, "value or values required")
		return
	}
	mode := domain.TargetMode(in.Mode)
	if mode == "" {
		mode = domain.ModePassiveOnly
	}
	var authBy *string
	var authAt *time.Time
	if in.Authorize && mode == domain.ModeActive {
		a := actor(r)
		now := time.Now()
		authBy = &a
		authAt = &now
	}
	var out []domain.ScopeTarget
	for _, v := range values {
		kind := in.Kind
		if kind == "" {
			kind = guessKind(v)
		}
		t := domain.ScopeTarget{
			ScopeID: scopeID, Kind: kind, Value: v, Tags: in.Tags, Mode: mode,
			AuthorizedBy: authBy, AuthorizedAt: authAt,
		}
		saved, err := s.st.AddTarget(r.Context(), t)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, saved)
	}
	s.auditReq(r, "target.add", scopeID.String(), map[string]any{"count": len(out)})
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) listTargets(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuid.Parse(chi.URLParam(r, "scopeID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad scope id")
		return
	}
	list, err := s.st.ListTargets(r.Context(), scopeID, r.URL.Query().Get("tag"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}
