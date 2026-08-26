package mission

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

type ArchiveEvidenceFilter struct {
	GateCode  string
	RecordID  string
	ActorID   string
	EventType string
	Cursor    int64
	Limit     int
}

type ArchiveEvidenceItem struct {
	GateCode           string            `json:"gate_code"`
	RecordID           string            `json:"record_id"`
	RecordSummary      json.RawMessage   `json:"record_payload_summary"`
	Lineage            []json.RawMessage `json:"lineage,omitempty"`
	AuditSequence      int64             `json:"audit_sequence"`
	EventType          string            `json:"event_type"`
	ActorID            string            `json:"actor_id"`
	EventHash          string            `json:"event_hash"`
	PreviousHash       string            `json:"previous_hash"`
	ArchiveChainDigest string            `json:"archive_chain_digest"`
	ProofNodes         []audit.Event     `json:"proof_nodes"`
}

type ArchiveEvidenceResult struct {
	Evidence           []ArchiveEvidenceItem `json:"evidence"`
	Proof              [][]audit.Event       `json:"proof"`
	NextCursor         int64                 `json:"next_cursor,omitempty"`
	ArchiveChainDigest string                `json:"archive_chain_digest"`
}

type evidenceRecord struct {
	Gate, ID   string
	Value      any
	EventTypes map[string]bool
}

func archiveRecordLineage(m *DiveMission, value any) []json.RawMessage {
	record, ok := value.(VerificationRecord)
	if !ok || record.RecordType != "equipment" || record.ReplacementForAssetID == "" {
		return nil
	}
	all := append(append([]VerificationRecord(nil), m.VerificationHistory...), m.Verifications...)
	byAsset := map[string]VerificationRecord{}
	for _, candidate := range all {
		if candidate.RecordType == "equipment" {
			byAsset[candidate.AssetID] = candidate
		}
	}
	var lineage []json.RawMessage
	for asset := record.ReplacementForAssetID; asset != ""; {
		previous, exists := byAsset[asset]
		if !exists {
			break
		}
		encoded, err := json.Marshal(previous)
		if err != nil {
			break
		}
		lineage = append(lineage, encoded)
		asset = previous.ReplacementForAssetID
	}
	for left, right := 0, len(lineage)-1; left < right; left, right = left+1, right-1 {
		lineage[left], lineage[right] = lineage[right], lineage[left]
	}
	return lineage
}

func archiveRecords(m *DiveMission) []evidenceRecord {
	var out []evidenceRecord
	for _, q := range m.MemberQualifications {
		out = append(out, evidenceRecord{"qualifications", q.PersonID, q, map[string]bool{"mission_created": true, "draft_revised": true}})
	}
	for _, risk := range append(append([]SegmentRisk(nil), m.RiskHistory...), m.Risks...) {
		out = append(out, evidenceRecord{"risk_assessment", risk.ID, risk, map[string]bool{"risks_assessed": true, "risks_reassessed": true}})
		for _, action := range risk.MitigationActions {
			events := map[string]bool{"risks_assessed": true, "risks_reassessed": true}
			if action.Status == "completed" {
				events = map[string]bool{"mitigation_completed": true}
			}
			out = append(out, evidenceRecord{"risk_assessment", action.Code, action, events})
		}
	}
	plans := append([]LifeSupportPlanVersion(nil), m.PlanHistory...)
	if m.LifeSupportPlan != nil {
		plans = append(plans, LifeSupportPlanVersion{Plan: *m.LifeSupportPlan})
	}
	for _, version := range plans {
		out = append(out, evidenceRecord{"life_support_plan", version.Plan.ID, version.Plan, map[string]bool{"life_support_plan_submitted": true, "life_support_plan_revised": true}})
	}
	allVerifications := append(append([]VerificationRecord(nil), m.VerificationHistory...), m.Verifications...)
	for _, record := range allVerifications {
		gate, events := "drills", map[string]bool{"drill_batch_recorded": true, "remediation_batch_recorded": true, "retest_batch_recorded": true}
		if record.RecordType == "equipment" {
			gate, events = "equipment", map[string]bool{"equipment_verified": true, "equipment_replaced": true, "equipment_evidence_refreshed": true}
		}
		out = append(out, evidenceRecord{gate, record.ID, record, events})
	}
	return out
}

func eventContainsRecord(event audit.Event, recordID string) bool {
	var value any
	if json.Unmarshal(event.Data, &value) != nil {
		return false
	}
	var walk func(any) bool
	walk = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if (key == "id" || key == "record_id" || key == "code" || key == "action_code" || key == "person_id") && child == recordID {
					return true
				}
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}

func verifyFrozenRelease(m *DiveMission, events []audit.Event) bool {
	if m.ReleaseDigest == "" {
		return false
	}
	for _, event := range events {
		if event.EventType != "mission_signed" {
			continue
		}
		var payload struct {
			ReleaseDigest string `json:"release_digest"`
		}
		if json.Unmarshal(event.Data, &payload) == nil && payload.ReleaseDigest == m.ReleaseDigest {
			return true
		}
	}
	return false
}

func (s *Service) LocateArchiveEvidence(ctx context.Context, id string, filter ArchiveEvidenceFilter) (ArchiveEvidenceResult, error) {
	m, err := s.repo.Mission(ctx, id)
	if err != nil {
		return ArchiveEvidenceResult{}, err
	}
	if m.Status != StatusArchived {
		return ArchiveEvidenceResult{}, InvalidState(m.Status, "查询归档证据")
	}
	knownGates := map[string]bool{"qualifications": true, "risk_assessment": true, "life_support_plan": true, "equipment": true, "drills": true}
	if filter.GateCode != "" && !knownGates[filter.GateCode] {
		return ArchiveEvidenceResult{}, Invalid("gate_code", "未知归档门禁代码")
	}
	if filter.Cursor < 0 || filter.Limit <= 0 || filter.Limit > 200 {
		return ArchiveEvidenceResult{}, Invalid("cursor", "游标或 limit 超出范围")
	}
	events, err := s.repo.AllEvents(ctx, id)
	if err != nil || audit.Verify(events) != nil || len(events) < 2 || events[len(events)-1].EventType != "mission_archived" || events[len(events)-2].EventHash != m.ArchiveDigest || !verifyFrozenRelease(m, events) {
		return ArchiveEvidenceResult{}, NewError("archive_integrity_failed", "归档链、归档摘要或签发摘要校验失败", 409)
	}
	eventTypes := map[string]bool{}
	for _, event := range events {
		eventTypes[event.EventType] = true
	}
	if filter.EventType != "" && !eventTypes[filter.EventType] {
		return ArchiveEvidenceResult{}, Invalid("event_type", "未知审计事件类型")
	}
	records := archiveRecords(m)
	recordBelongs := false
	var matched []ArchiveEvidenceItem
	for _, record := range records {
		if filter.RecordID != "" && record.ID != filter.RecordID {
			continue
		}
		if record.ID == filter.RecordID {
			recordBelongs = true
		}
		if filter.GateCode != "" && record.Gate != filter.GateCode {
			continue
		}
		var source *audit.Event
		for i := range events {
			if record.EventTypes[events[i].EventType] && eventContainsRecord(events[i], record.ID) {
				source = &events[i]
				break
			}
		}
		if source == nil {
			continue
		}
		if filter.ActorID != "" && source.ActorID != filter.ActorID || filter.EventType != "" && source.EventType != filter.EventType {
			continue
		}
		summary, marshalErr := json.Marshal(record.Value)
		if marshalErr != nil {
			return ArchiveEvidenceResult{}, marshalErr
		}
		proof := append([]audit.Event(nil), events[source.Sequence-1:]...)
		matched = append(matched, ArchiveEvidenceItem{GateCode: record.Gate, RecordID: record.ID, RecordSummary: summary, Lineage: archiveRecordLineage(m, record.Value), AuditSequence: source.Sequence, EventType: source.EventType, ActorID: source.ActorID, EventHash: source.EventHash, PreviousHash: source.PreviousHash, ArchiveChainDigest: events[len(events)-1].EventHash, ProofNodes: proof})
	}
	if filter.RecordID != "" && !recordBelongs {
		return ArchiveEvidenceResult{}, NewError("evidence_not_found", "归档证据不存在", 404)
	}
	if filter.RecordID != "" && recordBelongs && len(matched) == 0 {
		return ArchiveEvidenceResult{}, Unprocessable("filters", "筛选条件相互矛盾", nil)
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].AuditSequence == matched[j].AuditSequence {
			return strings.Compare(matched[i].RecordID, matched[j].RecordID) < 0
		}
		return matched[i].AuditSequence < matched[j].AuditSequence
	})
	result := ArchiveEvidenceResult{ArchiveChainDigest: events[len(events)-1].EventHash}
	if filter.Cursor > int64(len(matched)) {
		return ArchiveEvidenceResult{}, Invalid("cursor", "游标超出证据结果范围")
	}
	remaining := matched[filter.Cursor:]
	if len(remaining) > filter.Limit {
		result.Evidence, result.NextCursor = remaining[:filter.Limit], filter.Cursor+int64(filter.Limit)
	} else {
		result.Evidence = remaining
	}
	for _, item := range result.Evidence {
		result.Proof = append(result.Proof, item.ProofNodes)
	}
	return result, nil
}
