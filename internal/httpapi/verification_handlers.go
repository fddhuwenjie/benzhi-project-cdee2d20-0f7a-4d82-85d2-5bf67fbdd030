package httpapi

import (
	"net/http"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func (s *Server) VerifyEquipmentHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		CheckCode             string                         `json:"check_code,omitempty"`
		Outcome               string                         `json:"outcome,omitempty"`
		EvidenceDigest        string                         `json:"evidence_digest,omitempty"`
		ReviewMarker          string                         `json:"review_marker,omitempty"`
		FailureReason         string                         `json:"failure_reason,omitempty"`
		ReplacementForAssetID string                         `json:"replacement_for_asset_id,omitempty"`
		ReplacementReason     string                         `json:"replacement_reason,omitempty"`
		AssetID               string                         `json:"asset_id,omitempty"`
		InspectedAt           time.Time                      `json:"inspected_at,omitempty"`
		ValidUntil            time.Time                      `json:"valid_until,omitempty"`
		Measurements          map[string]mission.Measurement `json:"measurements,omitempty"`
		Items                 []mission.EquipmentInput       `json:"items,omitempty"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	items := request.Items
	if len(items) == 0 {
		items = []mission.EquipmentInput{{CheckCode: request.CheckCode, Outcome: request.Outcome, EvidenceDigest: request.EvidenceDigest, ReviewMarker: request.ReviewMarker, FailureReason: request.FailureReason, ReplacementForAssetID: request.ReplacementForAssetID, ReplacementReason: request.ReplacementReason, AssetID: request.AssetID, InspectedAt: request.InspectedAt, ValidUntil: request.ValidUntil, Measurements: request.Measurements}}
	}
	for _, item := range items {
		if len(item.Measurements) == 0 {
			writeError(w, mission.Invalid("measurements", "装备核验必须提交对应量化读数"), request.RequestID)
			return
		}
	}
	input := mission.EquipmentBatchInput{Meta: request.writeMeta.commandMeta(), Items: items}
	result, replay, err := s.missions.VerifyEquipmentBatch(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}

func (s *Server) RecordDrillHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		mission.DrillInput
		Items []mission.DrillInput `json:"items,omitempty"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	items := request.Items
	if len(items) == 0 {
		items = []mission.DrillInput{request.DrillInput}
	}
	result, replay, err := s.missions.RecordDrillBatch(r.Context(), r.PathValue("id"), mission.DrillBatchInput{Meta: request.writeMeta.commandMeta(), Items: items})
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}

func (s *Server) RecordRemediationHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		mission.RemediationInput
		Items []mission.RemediationInput `json:"items,omitempty"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	items := request.Items
	if len(items) == 0 {
		items = []mission.RemediationInput{request.RemediationInput}
	}
	result, replay, err := s.missions.RecordRemediationBatch(r.Context(), r.PathValue("id"), mission.RemediationBatchInput{Meta: request.writeMeta.commandMeta(), Items: items})
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}

func (s *Server) RecordRetestHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		mission.RetestInput
		Items []mission.RetestInput `json:"items,omitempty"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	items := request.Items
	if len(items) == 0 {
		items = []mission.RetestInput{request.RetestInput}
	}
	result, replay, err := s.missions.RecordRetestBatch(r.Context(), r.PathValue("id"), mission.RetestBatchInput{Meta: request.writeMeta.commandMeta(), Items: items})
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}
