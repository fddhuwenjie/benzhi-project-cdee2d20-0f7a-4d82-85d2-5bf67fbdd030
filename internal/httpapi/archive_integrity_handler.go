package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func (s *Server) ArchiveIntegrityHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var from, to *time.Time
	for name, dst := range map[string]**time.Time{"archived_from": &from, "archived_to": &to} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeError(w, mission.Invalid(name, "必须为 RFC3339 时间"), "")
				return
			}
			*dst = &t
		}
	}
	if from == nil && q.Get("from") != "" {
		t, err := time.Parse(time.RFC3339, q.Get("from"))
		if err != nil {
			writeError(w, mission.Invalid("from", "必须为 RFC3339 时间"), "")
			return
		}
		from = &t
	}
	if to == nil && q.Get("to") != "" {
		t, err := time.Parse(time.RFC3339, q.Get("to"))
		if err != nil {
			writeError(w, mission.Invalid("to", "必须为 RFC3339 时间"), "")
			return
		}
		to = &t
	}
	if from != nil && to != nil && to.Before(*from) {
		writeError(w, mission.Invalid("archived_to", "归档时间范围不能倒置"), "")
		return
	}
	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 200 {
			writeError(w, mission.Invalid("limit", "必须在 1 到 200 之间"), "")
			return
		}
		limit = n
	}
	result, err := s.missions.InspectArchiveIntegrity(r.Context(), mission.ArchiveIntegrityFilter{CaveSite: q.Get("cave_site"), From: from, To: to, Limit: limit, Cursor: q.Get("cursor")})
	if err != nil {
		writeError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
