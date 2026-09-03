package api

import (
	"net/http"
	"time"
)

func (s *Server) handleCurrentActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.semantic == nil {
		serviceUnavailable(w)
		return
	}
	jsonOK(w, s.semantic.Current(time.Now()))
}
