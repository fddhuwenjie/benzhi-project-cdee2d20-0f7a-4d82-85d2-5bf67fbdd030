package mission

import (
	"encoding/json"
	"sort"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

func jsonContainsString(value any, wanted string) bool {
	switch typed := value.(type) {
	case string:
		return typed == wanted
	case []any:
		for _, item := range typed {
			if jsonContainsString(item, wanted) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if jsonContainsString(item, wanted) {
				return true
			}
		}
	}
	return false
}

func eventForRecord(events []audit.Event, recordID string) (audit.Event, bool) {
	for _, event := range events {
		var payload any
		if json.Unmarshal(event.Data, &payload) == nil && jsonContainsString(payload, recordID) {
			return event, true
		}
	}
	return audit.Event{}, false
}

func evidenceReference(events []audit.Event, gate, code, id, actor string, occurred time.Time, relationship string) (EvidenceReference, error) {
	event, ok := eventForRecord(events, id)
	if !ok {
		return EvidenceReference{}, NewError("audit_integrity_failed", "当前门禁记录缺少对应成功审计事件", 409)
	}
	if actor == "" {
		actor = event.ActorID
	}
	if occurred.IsZero() {
		occurred = event.OccurredAt
	}
	return EvidenceReference{GateCode: gate, RecordCode: code, RecordID: id, BusinessVersion: event.ToRevision, ActorID: actor, OccurredAt: occurred, AuditSequence: event.Sequence, Relationship: relationship}, nil
}

func traceReleaseChecklist(m *DiveMission, events []audit.Event) ([]ChecklistItem, error) {
	if err := audit.Verify(events); err != nil {
		return nil, NewError("audit_integrity_failed", "放行证据审计链校验失败", 409)
	}
	items := BuildReleaseChecklist(m)
	byCode := map[string]*ChecklistItem{}
	for i := range items {
		byCode[items[i].Code] = &items[i]
	}
	for _, risk := range m.Risks {
		ref, err := evidenceReference(events, "risk_assessment", risk.SegmentName, risk.ID, risk.AssessedBy, time.Time{}, "current")
		if err != nil {
			return nil, err
		}
		byCode["risk_assessment"].SourceRecords = append(byCode["risk_assessment"].SourceRecords, ref)
	}
	if m.LifeSupportPlan != nil {
		ref, err := evidenceReference(events, "life_support_plan", "plan", m.LifeSupportPlan.ID, m.LifeSupportPlan.ReviewedBy, time.Time{}, "current")
		if err != nil {
			return nil, err
		}
		byCode["life_support_plan"].SourceRecords = append(byCode["life_support_plan"].SourceRecords, ref)
	}
	for _, record := range m.Verifications {
		gate := ""
		if record.RecordType == "equipment" {
			gate = "equipment"
		}
		if record.RecordType == "drill" || record.RecordType == "retest" {
			gate = "drills"
		}
		if gate == "" {
			continue
		}
		ref, err := evidenceReference(events, gate, record.CheckCode, record.ID, record.VerifiedBy, record.RecordedAt, "current")
		if err != nil {
			return nil, err
		}
		byCode[gate].SourceRecords = append(byCode[gate].SourceRecords, ref)
	}
	for _, record := range m.VerificationHistory {
		gate := "equipment"
		relationship := "superseded"
		if record.RecordType != "equipment" {
			gate, relationship = "drills", "cycle_history"
		}
		ref, err := evidenceReference(events, gate, record.CheckCode, record.ID, record.VerifiedBy, record.RecordedAt, relationship)
		if err != nil {
			return nil, err
		}
		byCode[gate].Lineage = append(byCode[gate].Lineage, ref)
	}
	actions := map[string][2]string{"risk_assessment": {"submit_risks", "/api/v1/dive-missions/{id}/risks"}, "life_support_plan": {"submit_or_review_life_support_plan", "/api/v1/dive-missions/{id}/life-support-plan"}, "equipment": {"verify_equipment", "/api/v1/dive-missions/{id}/equipment-verifications"}, "drills": {"record_drill_or_close_remediation", "/api/v1/dive-missions/{id}/drills"}}
	for i := range items {
		if !items[i].Passed {
			items[i].RequiredAction, items[i].Endpoint = actions[items[i].Code][0], actions[items[i].Code][1]
		}
		sort.Slice(items[i].SourceRecords, func(a, b int) bool {
			x, y := items[i].SourceRecords[a], items[i].SourceRecords[b]
			if x.RecordCode == y.RecordCode {
				if x.BusinessVersion == y.BusinessVersion {
					return x.RecordID < y.RecordID
				}
				return x.BusinessVersion < y.BusinessVersion
			}
			return x.RecordCode < y.RecordCode
		})
		sort.Slice(items[i].Lineage, func(a, b int) bool {
			x, y := items[i].Lineage[a], items[i].Lineage[b]
			if x.RecordCode == y.RecordCode {
				if x.BusinessVersion == y.BusinessVersion {
					return x.RecordID < y.RecordID
				}
				return x.BusinessVersion < y.BusinessVersion
			}
			return x.RecordCode < y.RecordCode
		})
	}
	return items, nil
}
