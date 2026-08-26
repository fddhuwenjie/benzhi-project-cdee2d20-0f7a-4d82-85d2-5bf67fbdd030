package mission

import (
	"context"
	"sort"
	"strings"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

func releaseBaseline(m *DiveMission, checklist []ChecklistItem, confirmation any) any {
	risks := append([]SegmentRisk(nil), m.Risks...)
	sort.Slice(risks, func(i, j int) bool { return risks[i].SegmentName < risks[j].SegmentName })
	verifications := append([]VerificationRecord(nil), m.Verifications...)
	sort.Slice(verifications, func(i, j int) bool {
		if verifications[i].RecordType == verifications[j].RecordType {
			return verifications[i].CheckCode < verifications[j].CheckCode
		}
		return verifications[i].RecordType < verifications[j].RecordType
	})
	return struct {
		MissionID             string                `json:"mission_id"`
		Revision              int64                 `json:"revision"`
		Title                 string                `json:"title"`
		CaveSite              string                `json:"cave_site"`
		TargetDepthM          float64               `json:"target_depth_m"`
		WindowStart           string                `json:"window_start"`
		WindowEnd             string                `json:"window_end"`
		LeaderID              string                `json:"leader_id"`
		Segments              []string              `json:"segments"`
		MemberQualifications  []MemberQualification `json:"member_qualifications"`
		Risks                 []SegmentRisk         `json:"risks"`
		RiskHistory           []SegmentRisk         `json:"risk_history,omitempty"`
		Plan                  *LifeSupportPlan      `json:"life_support_plan"`
		Verifications         []VerificationRecord  `json:"verifications"`
		Checklist             []ChecklistItem       `json:"checklist"`
		RejectionConfirmation any                   `json:"rejection_confirmation,omitempty"`
	}{m.ID, m.Revision + 1, m.Title, m.CaveSite, m.TargetDepthM, m.WindowStart.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		m.WindowEnd.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), m.LeaderID, m.Segments, m.MemberQualifications, risks, m.RiskHistory, m.LifeSupportPlan, verifications, checklist, confirmation}
}

func isIndependentSupervisor(m *DiveMission, actor string) bool {
	return len(supervisorConflicts(m, actor)) == 0
}

func (s *Service) Release(ctx context.Context, id string, in ReleaseInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "release", in.Meta, in, func(ctx context.Context, tx Tx, m *DiveMission) (string, any, error) {
		events, eventsErr := tx.AllEvents(ctx, m.ID)
		if eventsErr != nil {
			return "", nil, eventsErr
		}
		checklist, traceErr := traceReleaseChecklist(m, events)
		if traceErr != nil {
			return "", nil, traceErr
		}
		if in.SourceRevision > 0 && in.SourceRevision != m.Revision {
			return "", nil, &Error{Code: "preview_conflict", Message: "放行预览已过期，请重新预览", Status: 409, Details: map[string]any{"current_revision": m.Revision}}
		}
		if in.PreviewDigest != "" {
			d, _ := commandFingerprint("release_preview", CommandMeta{ExpectedRevision: m.Revision}, struct {
				Checklist []ChecklistItem `json:"checklist"`
			}{checklist})
			if d != in.PreviewDigest {
				return "", nil, &Error{Code: "preview_conflict", Message: "放行预览摘要已过期，请重新预览", Status: 409}
			}
		}
		if !isIndependentSupervisor(m, in.Meta.ActorID) {
			return "", nil, Invalid("actor_id", "签发人必须独立于任务、审核和核验人员")
		}
		if in.Decision != "sign" && in.Decision != "reject" {
			return "", nil, Invalid("decision", "必须为 sign 或 reject")
		}
		if in.Decision == "reject" {
			if m.Status != StatusReadyForRelease && m.Status != StatusReleaseRejected {
				return "", nil, InvalidState(m.Status, "拒绝放行")
			}
			if strings.TrimSpace(in.Reason) == "" {
				return "", nil, Invalid("reason", "拒绝放行时必须填写理由")
			}
			previewDigest, _ := commandFingerprint("release_preview", CommandMeta{ExpectedRevision: m.Revision}, struct {
				Checklist []ChecklistItem `json:"checklist"`
			}{checklist})
			cycle := 1
			if m.LastReleaseRejection != nil {
				cycle = m.LastReleaseRejection.Cycle + 1
			}
			m.LastReleaseRejection = &ReleaseRejection{Reason: strings.TrimSpace(in.Reason), ActorID: in.Meta.ActorID, Revision: m.Revision + 1, Checklist: checklist, PreviewDigest: previewDigest, Cycle: cycle}
			m.Status = StatusReleaseRejected
			return "release_rejected", m.LastReleaseRejection, nil
		}
		if !checklistPassed(checklist) {
			return "", nil, &Error{Code: "release_gate_failed", Message: "放行门禁未全部满足", Status: 409, Details: map[string]any{"checklist": checklist}}
		}
		var confirmation any
		resolved, remaining := []string{}, []string{}
		cycle := 0
		if m.LastReleaseRejection != nil {
			rejection := m.LastReleaseRejection
			if rejection == nil {
				return "", nil, NewError("rejection_context_missing", "最近拒绝上下文不存在", 409)
			}
			if in.RejectionRevision == 0 {
				return "", nil, Invalid("rejection_revision", "拒绝后重新签发时为必填")
			}
			if in.PreviewDigest == "" {
				return "", nil, Invalid("preview_digest", "拒绝后重新签发时为必填")
			}
			if strings.TrimSpace(in.AcknowledgeReason) == "" {
				return "", nil, Invalid("acknowledge_reason", "拒绝后重新签发时为必填")
			}
			if in.RejectionRevision != rejection.Revision {
				return "", nil, &Error{Code: "preview_conflict", Message: "拒绝修订号已过期，请重新预览", Status: 409}
			}
			if m.Revision <= rejection.Revision {
				return "", nil, &Error{Code: "preview_conflict", Message: "拒绝后尚无可确认的门禁变更", Status: 409}
			}
			currentDigest, _ := commandFingerprint("release_preview", CommandMeta{ExpectedRevision: m.Revision}, struct {
				Checklist []ChecklistItem `json:"checklist"`
			}{checklist})
			if currentDigest != in.PreviewDigest || currentDigest == rejection.PreviewDigest {
				return "", nil, &Error{Code: "preview_conflict", Message: "放行清单未体现拒绝后的门禁变化", Status: 409}
			}
			current := map[string]ChecklistItem{}
			for _, item := range checklist {
				current[item.Code] = item
			}
			for _, old := range rejection.Checklist {
				if !old.Passed && current[old.Code].Passed {
					resolved = append(resolved, old.Code)
				} else if !old.Passed {
					remaining = append(remaining, old.Code)
				}
			}
			sort.Strings(resolved)
			sort.Strings(remaining)
			if len(remaining) > 0 {
				return "", nil, &Error{Code: "release_gate_failed", Message: "最近拒绝清单仍有未解决门禁", Status: 409, Details: map[string]any{"remaining_blockers": remaining}}
			}
			cycle = rejection.Cycle
			confirmation = map[string]any{"rejection_cycle": cycle, "rejection_revision": rejection.Revision, "previous_checklist": rejection.Checklist, "current_checklist": checklist, "resolved_blockers": resolved, "remaining_blockers": remaining, "acknowledge_reason": strings.TrimSpace(in.AcknowledgeReason)}
		}
		digest, err := audit.Digest(releaseBaseline(m, checklist, confirmation))
		if err != nil {
			return "", nil, err
		}
		m.ReleaseChecklist, m.ReleaseDigest, m.SignedBy, m.Status = checklist, digest, in.Meta.ActorID, StatusSigned
		m.LastReleaseRejection = nil
		return "mission_signed", struct {
			Checklist             []ChecklistItem `json:"checklist"`
			ReleaseDigest         string          `json:"release_digest"`
			RejectionCycle        int             `json:"rejection_cycle,omitempty"`
			ResolvedBlockers      []string        `json:"resolved_blockers"`
			RemainingBlockers     []string        `json:"remaining_blockers"`
			RejectionConfirmation any             `json:"rejection_confirmation,omitempty"`
		}{checklist, digest, cycle, resolved, remaining, confirmation}, nil
	})
}

func (s *Service) Archive(ctx context.Context, id string, in ArchiveInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "archive", in.Meta, in, func(ctx context.Context, tx Tx, m *DiveMission) (string, any, error) {
		if m.Status != StatusSigned {
			return "", nil, InvalidState(m.Status, "归档")
		}
		if m.ReleaseDigest == "" || !checklistPassed(m.ReleaseChecklist) {
			return "", nil, &Error{Code: "baseline_invalid", Message: "签发基线无效", Status: 409}
		}
		events, err := tx.AllEvents(ctx, m.ID)
		if err != nil {
			return "", nil, err
		}
		chainDigest, err := audit.ChainDigest(events)
		if err != nil {
			return "", nil, &Error{Code: "audit_integrity_failed", Message: "审计链完整性校验失败", Status: 409}
		}
		now := s.now().UTC()
		m.ArchiveDigest, m.Status, m.ArchivedAt = chainDigest, StatusArchived, &now
		return "mission_archived", struct {
			ReleaseDigest    string `json:"release_digest"`
			AuditChainDigest string `json:"audit_chain_digest"`
		}{m.ReleaseDigest, chainDigest}, nil
	})
}
