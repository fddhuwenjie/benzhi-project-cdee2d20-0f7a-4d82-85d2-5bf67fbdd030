package mission

import (
	"context"
	"sort"
	"strings"
)

func (s *Service) ReviseDraft(ctx context.Context, id string, in DraftRevisionInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "revise_draft", in.Meta, in, func(ctx context.Context, tx Tx, m *DiveMission) (string, any, error) {
		n, changed, err := ValidateDraftRevision(m, in, s.now().UTC())
		if err != nil {
			return "", nil, err
		}
		if len(changed) == 0 {
			return "", nil, Invalid("body", "至少提供一个修订字段")
		}
		qualifications := m.MemberQualifications
		if in.MemberQualifications != nil {
			qualifications = append([]MemberQualification(nil), (*in.MemberQualifications)...)
			changed = append(changed, "member_qualifications")
		}
		if len(qualifications) > 0 {
			canonical, statuses, qualificationErr := ValidateQualifications(n.TeamMembers, qualifications, n.TargetDepthM, n.WindowEnd)
			if qualificationErr != nil {
				return "", nil, qualificationErr
			}
			n.MemberQualifications, n.QualificationStatus = canonical, statuses
		}
		sort.Strings(changed)
		if n.CaveSiteKey != m.CaveSiteKey || !n.WindowStart.Equal(m.WindowStart) || !n.WindowEnd.Equal(m.WindowEnd) {
			conflicts, err := tx.ScheduleConflictsExcluding(ctx, n.CaveSiteKey, n.WindowStart, n.WindowEnd, m.ID)
			if err != nil {
				return "", nil, err
			}
			if len(conflicts) > 0 {
				return "", nil, &Error{Code: "schedule_conflict", Message: "任务时间窗与同站点未归档任务重叠", Status: 409, Details: map[string]any{"conflicts": conflicts}}
			}
		}
		m.Title, m.CaveSite, m.CaveSiteKey, m.TargetDepthM, m.WindowStart, m.WindowEnd, m.Segments, m.TeamMembers, m.LeaderID = n.Title, n.CaveSite, n.CaveSiteKey, n.TargetDepthM, n.WindowStart, n.WindowEnd, n.Segments, n.TeamMembers, n.LeaderID
		m.MemberQualifications, m.QualificationStatus = n.MemberQualifications, n.QualificationStatus
		return "draft_revised", map[string]any{"changed_fields": changed, "draft": m}, nil
	})
}

func (s *Service) ReassessRisks(ctx context.Context, id string, in RiskReassessmentInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "reassess_risks", in.Meta, in, func(_ context.Context, _ Tx, m *DiveMission) (string, any, error) {
		if m.Status != StatusRiskAssessed || m.LifeSupportPlan != nil {
			return "", nil, InvalidState(m.Status, "洞段风险复评")
		}
		if strings.TrimSpace(in.Reason) == "" {
			return "", nil, Invalid("reason", "复评必须填写原因")
		}
		risks, err := ValidateRisks(m, RiskInput{Risks: in.Risks})
		if err != nil {
			return "", nil, err
		}
		diffs := RiskDifferences(m.Risks, risks)
		if len(diffs) == 0 {
			return "", nil, Unprocessable("risks", "复评未产生指标或缓解措施变化", nil)
		}
		for i := range risks {
			risks[i].ID = newID("risk")
			risks[i].MissionID = m.ID
			risks[i].AssessedBy = in.Meta.ActorID
		}
		if hasStructuredMitigations(risks) {
			risks, err = validateMitigationActions(m, risks)
			if err != nil {
				return "", nil, err
			}
			linkMitigationVersions(m.Risks, risks)
		}
		m.RiskHistory = append(m.RiskHistory, m.Risks...)
		m.Risks, m.RiskSummary = risks, SummarizeRisks(risks)
		return "risks_reassessed", map[string]any{"reason": strings.TrimSpace(in.Reason), "differences": diffs, "summary": m.RiskSummary}, nil
	})
}
