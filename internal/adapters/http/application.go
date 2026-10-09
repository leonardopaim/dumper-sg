package httpapi

import "net/http"

func (s *Server) application(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"instance_id": s.instance, "restart_available": s.options.Restart != nil})
}

func (s *Server) restart(w http.ResponseWriter, r *http.Request) {
	if s.options.Restart == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Inicie a aplicação por scripts/start.ps1 ou pela inicialização automática para habilitar o reinício."})
		return
	}
	var body struct{}
	if err := decode(w, r, &body); err != nil {
		fail(w, err)
		return
	}
	if err := s.options.Restart(r.Context()); err != nil {
		fail(w, err)
		return
	}
	// Shutdown drains this HTTP response before the process exits.
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "restarting", "instance_id": s.instance})
}

func (s *Server) shutdown(w http.ResponseWriter, r *http.Request) {
	if s.options.Shutdown == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Encerramento disponível na aplicação instalada."})
		return
	}
	var body struct{}
	if err := decode(w, r, &body); err != nil {
		fail(w, err)
		return
	}
	if err := s.options.Shutdown(r.Context()); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping", "instance_id": s.instance})
}
