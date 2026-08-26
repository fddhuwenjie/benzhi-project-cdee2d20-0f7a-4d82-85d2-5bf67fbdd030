package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func (s *Server) TemplatePreviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var body struct {
			RequestID         string           `json:"request_id"`
			TemplateMissionID string           `json:"template_mission_id"`
			Title             string           `json:"title"`
			TeamMembers       []mission.Member `json:"team_members"`
			WindowStart       time.Time        `json:"window_start"`
			WindowEnd         time.Time        `json:"window_end"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, err, body.RequestID)
			return
		}
		if body.TemplateMissionID == "" {
			body.TemplateMissionID = r.PathValue("id")
		}
		result, e := s.missions.PreviewTemplate(r.Context(), mission.TemplatePreviewInput{RequestID: body.RequestID, TemplateMissionID: body.TemplateMissionID, Title: body.Title, TeamMembers: body.TeamMembers, WindowStart: body.WindowStart, WindowEnd: body.WindowEnd})
		if e != nil {
			writeError(w, e, body.RequestID)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	q := r.URL.Query()
	templateID := q.Get("template_mission_id")
	if templateID == "" {
		templateID = r.PathValue("id")
	}
	start, e := time.Parse(time.RFC3339, q.Get("window_start"))
	if e != nil {
		writeError(w, mission.Invalid("window_start", "必须为 RFC3339 时间"), q.Get("request_id"))
		return
	}
	end, e := time.Parse(time.RFC3339, q.Get("window_end"))
	if e != nil {
		writeError(w, mission.Invalid("window_end", "必须为 RFC3339 时间"), q.Get("request_id"))
		return
	}
	result, e := s.missions.PreviewTemplate(r.Context(), mission.TemplatePreviewInput{RequestID: q.Get("request_id"), TemplateMissionID: templateID, Title: q.Get("title"), WindowStart: start, WindowEnd: end})
	if e != nil {
		writeError(w, e, q.Get("request_id"))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) MitigationQueryHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var from, to *time.Time
	if v := q.Get("due_from"); v != "" {
		t, e := time.Parse(time.RFC3339, v)
		if e != nil {
			writeError(w, mission.Invalid("due_from", "必须为 RFC3339 时间"), "")
			return
		}
		from = &t
	}
	if v := q.Get("due_to"); v != "" {
		t, e := time.Parse(time.RFC3339, v)
		if e != nil {
			writeError(w, mission.Invalid("due_to", "必须为 RFC3339 时间"), "")
			return
		}
		to = &t
	}
	limit := 50
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	res, e := s.missions.QueryMitigations(r.Context(), r.PathValue("id"), mission.MitigationQueryFilter{Segment: q.Get("segment"), Owner: q.Get("owner"), Status: q.Get("status"), From: from, To: to, Cursor: q.Get("cursor"), Limit: limit})
	if e != nil {
		writeError(w, e, "")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) EquipmentEvidenceHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	res, e := s.missions.QueryEquipmentEvidence(r.Context(), r.PathValue("id"), mission.EquipmentEvidenceFilter{Status: q.Get("status"), CheckCode: q.Get("check_code"), AssetID: q.Get("asset_id"), Cursor: q.Get("cursor"), Limit: limit})
	if e != nil {
		writeError(w, e, "")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) RemediationReviewHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cycle := 0
	if v := q.Get("cycle"); v != "" {
		cycle, _ = strconv.Atoi(v)
	}
	var overdue *bool
	if v := q.Get("overdue"); v != "" {
		b := v == "true"
		overdue = &b
	}
	res, e := s.missions.QueryRemediationReview(r.Context(), r.PathValue("id"), mission.RemediationReviewFilter{CheckCode: q.Get("check_code"), Cycle: cycle, Status: q.Get("status"), Overdue: overdue, Cursor: q.Get("cursor"), Limit: func() int {
		if v := q.Get("limit"); v != "" {
			n, _ := strconv.Atoi(v)
			return n
		}
		return 50
	}()})
	if e != nil {
		writeError(w, e, "")
		return
	}
	writeJSON(w, http.StatusOK, res)
}
