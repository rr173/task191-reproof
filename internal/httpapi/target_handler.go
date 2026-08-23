package httpapi

import (
	"net/http"
)

func (s *Server) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	var b reqBody
	if err := decodeBody(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	t, err := s.app.CreateTarget(r.Context(), b.Name, b.Description)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	ts, err := s.app.ListTargets(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

func (s *Server) handleGetTarget(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	t, err := s.app.GetTarget(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleAddSeed(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var b reqBody
	if err := decodeBody(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.AddSeed(r.Context(), id, b.Path); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"seed": b.Path, "target_id": id})
}

func (s *Server) handleListSeeds(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	seeds, err := s.app.ListSeeds(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, seeds)
}
