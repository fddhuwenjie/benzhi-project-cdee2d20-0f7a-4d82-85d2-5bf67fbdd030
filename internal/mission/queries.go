package mission

import (
	"context"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

func validateScheduleWindow(site string, start, end time.Time) error {
	if strings.TrimSpace(site) == "" {
		return Invalid("cave_site", "不能为空")
	}
	if start.IsZero() {
		return Invalid("window_start", "必须为 RFC3339 时间")
	}
	if end.IsZero() {
		return Invalid("window_end", "必须为 RFC3339 时间")
	}
	if !end.After(start) {
		return Invalid("window_end", "必须晚于 window_start")
	}
	if end.Sub(start) > 72*time.Hour {
		return Invalid("window_end", "任务时间窗不能超过 72 小时")
	}
	return nil
}

func scheduleDigest(siteKey string, start, end time.Time, conflicts []ScheduleConflict) (string, error) {
	return audit.Digest(struct {
		CaveSiteKey string             `json:"cave_site_key"`
		WindowStart string             `json:"window_start"`
		WindowEnd   string             `json:"window_end"`
		Conflicts   []ScheduleConflict `json:"conflicts"`
	}{siteKey, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano), conflicts})
}

func (s *Service) PreflightSchedule(ctx context.Context, requestID, site string, start, end time.Time) (SchedulePreflight, error) {
	if strings.TrimSpace(requestID) == "" {
		return SchedulePreflight{}, Invalid("request_id", "不能为空")
	}
	if err := validateScheduleWindow(site, start, end); err != nil {
		return SchedulePreflight{}, err
	}
	key := canonicalSiteKey(site)
	conflicts, err := s.repo.SchedulePreflight(ctx, key, start.UTC(), end.UTC())
	if err != nil {
		return SchedulePreflight{}, err
	}
	if conflicts == nil {
		conflicts = []ScheduleConflict{}
	}
	digest, err := scheduleDigest(key, start, end, conflicts)
	if err != nil {
		return SchedulePreflight{}, err
	}
	return SchedulePreflight{RequestID: strings.TrimSpace(requestID), ReadOnly: true, Available: len(conflicts) == 0, CaveSiteKey: key, WindowStart: start.UTC(), WindowEnd: end.UTC(), Conflicts: conflicts, SourceDigest: digest}, nil
}

type PlanVersionSummary struct {
	PlanID            string   `json:"plan_id"`
	Version           int64    `json:"version"`
	ReviewedBy        string   `json:"reviewed_by,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	FailedRules       []string `json:"failed_rules"`
	SubmittedRevision int64    `json:"submitted_revision"`
}

type PlanFieldDiff struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}
type FailedRuleCoverage struct {
	Covered         []string `json:"covered"`
	Uncovered       []string `json:"uncovered"`
	CoveragePercent float64  `json:"coverage_percent"`
	Applicable      bool     `json:"applicable"`
}
type PlanComparison struct {
	Current            *LifeSupportPlan     `json:"current"`
	Versions           []PlanVersionSummary `json:"versions"`
	Diff               []PlanFieldDiff      `json:"diff"`
	FailedRuleCoverage FailedRuleCoverage   `json:"failed_rule_coverage"`
	SourceRevision     int64                `json:"source_revision"`
}

func normalizedFloat(v float64) float64 { return math.Round(v*100) / 100 }
func normalizedGases(in []GasMix) []GasMix {
	out := append([]GasMix(nil), in...)
	for i := range out {
		out[i].OxygenPercent = normalizedFloat(out[i].OxygenPercent)
		out[i].HeliumPercent = normalizedFloat(out[i].HeliumPercent)
		out[i].CylinderLiters = normalizedFloat(out[i].CylinderLiters)
		out[i].IntendedDepthM = normalizedFloat(out[i].IntendedDepthM)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func sortedMembers(in []Member) []Member {
	out := append([]Member(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role == out[j].Role {
			return out[i].PersonID < out[j].PersonID
		}
		return out[i].Role < out[j].Role
	})
	return out
}
func sortedAssignments(in []SupportAssignment) []SupportAssignment {
	out := append([]SupportAssignment(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Duty == out[j].Duty {
			return out[i].PersonID < out[j].PersonID
		}
		return out[i].Duty < out[j].Duty
	})
	return out
}

func comparePlans(before, after *LifeSupportPlan) []PlanFieldDiff {
	var out []PlanFieldDiff
	add := func(field string, a, b any) {
		if !reflect.DeepEqual(a, b) {
			out = append(out, PlanFieldDiff{field, a, b})
		}
	}
	add("member_roles", sortedMembers(before.Members), sortedMembers(after.Members))
	add("gas_mixes", normalizedGases(before.GasMixes), normalizedGases(after.GasMixes))
	add("start_pressures", func() map[string]int {
		m := map[string]int{}
		for _, g := range before.GasMixes {
			m[g.Name] = g.StartPressureBar
		}
		return m
	}(), func() map[string]int {
		m := map[string]int{}
		for _, g := range after.GasMixes {
			m[g.Name] = g.StartPressureBar
		}
		return m
	}())
	add("turn_pressure_bar", before.TurnPressureBar, after.TurnPressureBar)
	add("reserve_rule", before.ReserveRule, after.ReserveRule)
	add("support_assignments", sortedAssignments(before.SupportAssignments), sortedAssignments(after.SupportAssignments))
	return out
}

func (s *Service) ComparePlan(ctx context.Context, missionID, planID string, compareTo int64) (PlanComparison, error) {
	m, err := s.repo.Mission(ctx, missionID)
	if err != nil {
		return PlanComparison{}, err
	}
	if m.LifeSupportPlan == nil {
		return PlanComparison{}, NewError("plan_not_found", "生命支持方案不存在", 404)
	}
	belongs := m.LifeSupportPlan.ID == planID
	for _, version := range m.PlanHistory {
		if version.Plan.ID == planID {
			belongs = true
			break
		}
	}
	if !belongs {
		return PlanComparison{}, NewError("plan_not_found", "方案不属于该任务", 404)
	}
	events, eventErr := s.repo.AllEvents(ctx, missionID)
	if eventErr != nil || audit.Verify(events) != nil {
		code, message := "audit_integrity_failed", "方案审计链完整性校验失败"
		if m.Status == StatusArchived {
			code, message = "archive_integrity_failed", "归档完整性校验失败"
		}
		return PlanComparison{}, NewError(code, message, 409)
	}
	versions := append([]LifeSupportPlanVersion(nil), m.PlanHistory...)
	if m.LifeSupportPlan.ReviewStatus == "rejected" {
		versions = append(versions, LifeSupportPlanVersion{Plan: *m.LifeSupportPlan, Version: m.LifeSupportPlan.Version, RejectedBy: m.LifeSupportPlan.ReviewedBy, Reason: m.LifeSupportPlan.ReviewNote, Revision: m.Revision - 1})
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].Version < versions[j].Version })
	summaries := make([]PlanVersionSummary, 0, len(versions))
	for _, v := range versions {
		summaries = append(summaries, PlanVersionSummary{v.Plan.ID, v.Version, v.RejectedBy, v.Reason, append([]string(nil), v.Plan.FailedRules...), v.Revision})
	}
	result := PlanComparison{Current: m.LifeSupportPlan, Versions: summaries, SourceRevision: m.Revision, Diff: []PlanFieldDiff{}, FailedRuleCoverage: FailedRuleCoverage{Covered: []string{}, Uncovered: []string{}, CoveragePercent: 100, Applicable: false}}
	if len(versions) == 0 {
		return result, nil
	}
	base := &versions[len(versions)-1].Plan
	if compareTo > 0 {
		base = nil
		for i := range versions {
			if versions[i].Version == compareTo {
				base = &versions[i].Plan
				break
			}
		}
		if base == nil {
			return PlanComparison{}, NewError("plan_version_invalid", "对照方案版本不存在", 422)
		}
	}
	result.Diff = comparePlans(base, m.LifeSupportPlan)
	result.FailedRuleCoverage.Applicable = true
	for _, rule := range base.FailedRules {
		if strings.TrimSpace(m.LifeSupportPlan.RemediationNotes[rule]) != "" {
			result.FailedRuleCoverage.Covered = append(result.FailedRuleCoverage.Covered, rule)
		} else {
			result.FailedRuleCoverage.Uncovered = append(result.FailedRuleCoverage.Uncovered, rule)
		}
	}
	sort.Strings(result.FailedRuleCoverage.Covered)
	sort.Strings(result.FailedRuleCoverage.Uncovered)
	total := len(result.FailedRuleCoverage.Covered) + len(result.FailedRuleCoverage.Uncovered)
	if total > 0 {
		result.FailedRuleCoverage.CoveragePercent = normalizedFloat(float64(len(result.FailedRuleCoverage.Covered)) * 100 / float64(total))
	}
	return result, nil
}
