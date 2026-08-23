package httpapi

import (
	"net/http"
	"strings"

	"task191-reproof/internal/model"
)

func (s *Server) handleCreateAction(w http.ResponseWriter, r *http.Request) {
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
	a, err := s.app.CreateAction(r.Context(), id, b.Name, b.Command)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleListActions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	acts, err := s.app.ListActions(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, acts)
}

func (s *Server) handleListDeclarations(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	decls, err := s.app.ListDeclarations(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decls)
}

func (s *Server) handleGetAction(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	a, err := s.app.GetAction(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleAddDep(w http.ResponseWriter, r *http.Request) {
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
	d, err := s.app.AddDep(r.Context(), id, b.DependsOn)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleAddDeclaration(w http.ResponseWriter, r *http.Request) {
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
	dir := model.AccessDirection(strings.ToLower(b.Direction))
	d, err := s.app.AddDeclaration(r.Context(), id, b.Path, dir, b.Kind)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleAddToolchain(w http.ResponseWriter, r *http.Request) {
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
	t, err := s.app.AddToolchain(r.Context(), id, b.Name, b.Version, b.Checksum)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleAppendLogs(w http.ResponseWriter, r *http.Request) {
	var b reqBody
	if err := decodeBody(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	entries := make([]model.LogEntry, 0, len(b.Logs))
	for _, lg := range b.Logs {
		entries = append(entries, model.LogEntry{
			ActionID:    lg.ActionID,
			Seq:         lg.Seq,
			Path:        lg.Path,
			Direction:   model.AccessDirection(strings.ToLower(lg.Direction)),
			ContentHash: lg.Hash,
			SizeBytes:   lg.Size,
		})
	}
	n, err := s.app.AppendLogs(r.Context(), entries)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"appended": n})
}

func (s *Server) handleListActionLogs(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	logs, err := s.app.ListLogs(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, logs)
}
