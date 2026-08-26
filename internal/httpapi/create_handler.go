package httpapi

import (
	"net/http"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func (s *Server) SchedulePreflightHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RequestID   string `json:"request_id"`
		CaveSite    string `json:"cave_site"`
		WindowStart string `json:"window_start"`
		WindowEnd   string `json:"window_end"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	start, err := time.Parse(time.RFC3339, request.WindowStart)
	if err != nil {
		writeError(w, mission.Invalid("window_start", "必须为 RFC3339 时间"), request.RequestID)
		return
	}
	end, err := time.Parse(time.RFC3339, request.WindowEnd)
	if err != nil {
		writeError(w, mission.Invalid("window_end", "必须为 RFC3339 时间"), request.RequestID)
		return
	}
	result, err := s.missions.PreflightSchedule(r.Context(), request.RequestID, request.CaveSite, start, end)
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type createRequest struct {
	writeMeta
	mission.CreateInput
}

func (s *Server) CreateMissionHandler(w http.ResponseWriter, r *http.Request) {
	var request createRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	if len(request.MemberQualifications) == 0 {
		writeError(w, mission.Invalid("member_qualifications", "必须为每名任务成员提交岗位资格快照"), request.RequestID)
		return
	}
	request.CreateInput.Meta = request.writeMeta.commandMeta()
	result, replay, err := s.missions.Create(r.Context(), request.CreateInput)
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}

func (s *Server) SubmitRisksHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		mission.RiskInput
		Reason string `json:"reason,omitempty"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	request.RiskInput.Meta = request.writeMeta.commandMeta()
	for _, risk := range request.Risks {
		if mission.RequiresMitigationActions(risk) && len(risk.MitigationActions) == 0 {
			writeError(w, mission.Unprocessable("mitigation_actions", "高风险洞段必须为每个危险项提交结构化缓解行动", map[string]any{"segment_name": risk.SegmentName}), request.RequestID)
			return
		}
	}
	if request.ValidateOnly || r.URL.Query().Get("validate_only") == "true" {
		preview, previewErr := s.missions.PreviewRisks(r.Context(), r.PathValue("id"), request.Risks)
		if previewErr != nil {
			writeError(w, previewErr, request.RequestID)
			return
		}
		writeJSON(w, http.StatusOK, preview)
		return
	}
	var result mission.StoredResult
	var replay bool
	var err error
	if request.Reason != "" {
		result, replay, err = s.missions.ReassessRisks(r.Context(), r.PathValue("id"), mission.RiskReassessmentInput{Meta: request.RiskInput.Meta, Reason: request.Reason, Risks: request.Risks})
	} else {
		result, replay, err = s.missions.SubmitRisks(r.Context(), r.PathValue("id"), request.RiskInput)
	}
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}

func (s *Server) ReviseDraftHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		writeMeta
		mission.DraftRevisionInput
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err, req.RequestID)
		return
	}
	req.DraftRevisionInput.Meta = req.writeMeta.commandMeta()
	result, replay, err := s.missions.ReviseDraft(r.Context(), r.PathValue("id"), req.DraftRevisionInput)
	if err != nil {
		writeError(w, err, req.RequestID)
		return
	}
	writeStored(w, result, replay, req.RequestID)
}

func (s *Server) ReassessRisksHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		writeMeta
		mission.RiskReassessmentInput
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err, req.RequestID)
		return
	}
	for _, risk := range req.Risks {
		if mission.RequiresMitigationActions(risk) && len(risk.MitigationActions) == 0 {
			writeError(w, mission.Unprocessable("mitigation_actions", "高风险洞段必须为每个危险项提交结构化缓解行动", map[string]any{"segment_name": risk.SegmentName}), req.RequestID)
			return
		}
	}
	req.RiskReassessmentInput.Meta = req.writeMeta.commandMeta()
	result, replay, err := s.missions.ReassessRisks(r.Context(), r.PathValue("id"), req.RiskReassessmentInput)
	if err != nil {
		writeError(w, err, req.RequestID)
		return
	}
	writeStored(w, result, replay, req.RequestID)
}

func (s *Server) CompleteMitigationHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		Result         string    `json:"result"`
		EvidenceDigest string    `json:"evidence_digest"`
		CompletedAt    time.Time `json:"completed_at,omitempty"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	input := mission.MitigationCompletionInput{Meta: request.writeMeta.commandMeta(), ActionCode: r.PathValue("action_code"), Result: request.Result, EvidenceDigest: request.EvidenceDigest, CompletedAt: request.CompletedAt}
	result, replay, err := s.missions.CompleteMitigation(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}

func (s *Server) CompleteMitigationBatchHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		Items []mission.MitigationBatchItem `json:"items"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	if len(request.Items) == 0 {
		writeError(w, mission.Invalid("items", "至少提交一项行动"), request.RequestID)
		return
	}
	in := mission.MitigationBatchInput{Meta: request.writeMeta.commandMeta(), Items: request.Items}
	result, replay, err := s.missions.CompleteMitigationBatch(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}
