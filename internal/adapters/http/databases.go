package httpapi

import (
	"net/http"

	"dumpersg/internal/core"
)

func (s *Server) listDatabases(w http.ResponseWriter, r *http.Request) {
	var req core.DatabaseListRequest
	if err := decode(w, r, &req); err != nil {
		fail(w, err)
		return
	}
	job, err := s.ops.StartDatabaseList(r.Context(), req)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) databaseResults(w http.ResponseWriter, r *http.Request) {
	result, err := s.ops.Databases(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
