package httpapi

import (
	"net/http"

	"dumpersg/internal/core"
)

func (s *Server) listTables(w http.ResponseWriter, r *http.Request) {
	var req core.TableListRequest
	if err := decode(w, r, &req); err != nil {
		fail(w, err)
		return
	}
	job, err := s.ops.StartTableList(r.Context(), req)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) tableResults(w http.ResponseWriter, r *http.Request) {
	tables, err := s.ops.Tables(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tables)
}

func (s *Server) backupTables(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BackupDir string `json:"backup_dir"`
	}
	if err := decode(w, r, &req); err != nil {
		fail(w, err)
		return
	}
	tables, err := core.ListBackupTables(req.BackupDir)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tables)
}
