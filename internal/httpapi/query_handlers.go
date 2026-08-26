package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func (s *Server) ListMissionsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	valid := map[string]bool{"draft": true, "risk_assessed": true, "plan_review": true, "equipment_verification": true, "drill_pending": true, "remediation": true, "ready_for_release": true, "release_rejected": true, "signed": true, "archived": true}
	status := q.Get("status")
	if status != "" && !valid[status] {
		writeError(w, mission.Invalid("status", "未知任务状态"), "")
		return
	}
	riskLevels := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	riskLevel := q.Get("risk_level")
	if riskLevel != "" && !riskLevels[riskLevel] {
		writeError(w, mission.Invalid("risk_level", "未知风险等级"), "")
		return
	}
	mitigationStates := map[string]bool{"complete": true, "incomplete": true, "none": true}
	mitigationState := q.Get("mitigation_state")
	if mitigationState != "" && !mitigationStates[mitigationState] {
		writeError(w, mission.Invalid("mitigation_state", "未知缓解门禁状态"), "")
		return
	}
	minScore, e := parseNonNegative(q.Get("min_total_score"), 0)
	if e != nil {
		writeError(w, mission.Invalid("min_total_score", "必须为非负整数"), "")
		return
	}
	parse := func(name string) (*time.Time, error) {
		v := q.Get(name)
		if v == "" {
			return nil, nil
		}
		t, e := time.Parse(time.RFC3339, v)
		if e != nil {
			return nil, mission.Invalid(name, "必须为 RFC3339 时间")
		}
		return &t, nil
	}
	from, e := parse("window_from")
	if e != nil {
		writeError(w, e, "")
		return
	}
	to, e := parse("window_to")
	if e != nil {
		writeError(w, e, "")
		return
	}
	if from != nil && to != nil && to.Before(*from) {
		writeError(w, mission.Invalid("window_to", "不能早于 window_from"), "")
		return
	}
	limit, e := parseNonNegative(q.Get("limit"), 50)
	if e != nil || limit == 0 {
		writeError(w, fmtQueryError(), "")
		return
	}
	result, e := s.missions.List(r.Context(), mission.ListFilter{Status: status, CaveSite: q.Get("cave_site"), LeaderID: q.Get("leader_id"), WindowFrom: from, WindowTo: to, Limit: limit, Cursor: q.Get("cursor"), RiskLevel: riskLevel, MinTotalScore: minScore, MitigationState: mitigationState})
	if e != nil {
		writeError(w, e, "")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) HealthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) GetMissionHandler(w http.ResponseWriter, r *http.Request) {
	m, err := s.missions.Mission(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mission": m, "allowed_actions": mission.AllowedActions(m)})
}

func (s *Server) GetHistoryHandler(w http.ResponseWriter, r *http.Request) {
	history, err := s.missions.History(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": history})
}

func (s *Server) GetAuditEventsHandler(w http.ResponseWriter, r *http.Request) {
	after, err := parseNonNegative(r.URL.Query().Get("after"), 0)
	if err != nil {
		writeError(w, err, "")
		return
	}
	limit, err := parseNonNegative(r.URL.Query().Get("limit"), 50)
	if err != nil {
		writeError(w, err, "")
		return
	}
	page, err := s.audit.Page(r.Context(), r.PathValue("id"), audit.Filter{After: int64(after), Limit: limit, EventType: r.URL.Query().Get("event_type"), StatusAfter: r.URL.Query().Get("status_after")})
	if err != nil {
		if mission.AsError(err).Code == "internal_error" {
			err = mission.NewError("audit_integrity_failed", "审计链完整性校验失败", http.StatusConflict)
		}
		writeError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func parseNonNegative(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmtQueryError()
	}
	return parsed, nil
}
