package httpapi

import (
	"net/http"
	"strconv"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func (s *Server) GetPlanHandler(w http.ResponseWriter, r *http.Request) {
	planID := r.PathValue("plan_id")
	if planID == "" {
		planID = r.URL.Query().Get("plan_id")
	}
	if planID == "" {
		writeError(w, mission.Invalid("plan_id", "不能为空"), "")
		return
	}
	compareTo := int64(0)
	if raw := r.URL.Query().Get("compare_to"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			writeError(w, mission.Invalid("compare_to", "必须为正整数版本号"), "")
			return
		}
		compareTo = v
	}
	result, err := s.missions.ComparePlan(r.Context(), r.PathValue("id"), planID, compareTo)
	if err != nil {
		writeError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) SubmitPlanHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		mission.PlanInput
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	if len(request.SegmentGasBudgets) == 0 {
		writeError(w, mission.Invalid("segment_gas_budgets", "必须提交完整的逐洞段失气撤离预算输入"), request.RequestID)
		return
	}
	request.PlanInput.Meta = request.writeMeta.commandMeta()
	result, replay, err := s.missions.SubmitPlan(r.Context(), r.PathValue("id"), request.PlanInput)
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}

func (s *Server) ReviewPlanHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		mission.ReviewInput
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	request.ReviewInput.Meta = request.writeMeta.commandMeta()
	result, replay, err := s.missions.ReviewPlan(r.Context(), r.PathValue("id"), request.ReviewInput)
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}
