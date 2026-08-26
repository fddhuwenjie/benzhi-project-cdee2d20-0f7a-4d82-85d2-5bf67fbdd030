package mission

import (
	"reflect"
	"sort"
	"strings"
	"time"
)

func ValidateDraftRevision(m *DiveMission, in DraftRevisionInput, now time.Time) (*DiveMission, []string, error) {
	if m.Status != StatusDraft {
		return nil, nil, InvalidState(m.Status, "修订任务草案")
	}
	n := *m
	if in.Title != nil {
		n.Title = strings.TrimSpace(*in.Title)
	}
	if in.CaveSite != nil {
		n.CaveSite = strings.TrimSpace(*in.CaveSite)
		n.CaveSiteKey = canonicalSiteKey(n.CaveSite)
	}
	if in.TargetDepthM != nil {
		n.TargetDepthM = *in.TargetDepthM
	}
	if in.WindowStart != nil {
		n.WindowStart = in.WindowStart.UTC()
	}
	if in.WindowEnd != nil {
		n.WindowEnd = in.WindowEnd.UTC()
	}
	if in.Segments != nil {
		n.Segments = append([]string(nil), (*in.Segments)...)
	}
	if in.TeamMembers != nil {
		n.TeamMembers = append([]Member(nil), (*in.TeamMembers)...)
	}
	if in.MemberQualifications != nil {
		n.MemberQualifications = append([]MemberQualification(nil), (*in.MemberQualifications)...)
	}
	check := CreateInput{Meta: CommandMeta{ExpectedRevision: 0, RequestID: "r", ActorID: "a"}, Title: n.Title, CaveSite: n.CaveSite, TargetDepthM: n.TargetDepthM, WindowStart: n.WindowStart, WindowEnd: n.WindowEnd, Segments: n.Segments, TeamMembers: n.TeamMembers, MemberQualifications: n.MemberQualifications}
	if err := ValidateCreate(check, now); err != nil {
		return nil, nil, err
	}
	n.LeaderID = ""
	for i := range n.Segments {
		n.Segments[i] = strings.TrimSpace(n.Segments[i])
	}
	for _, member := range n.TeamMembers {
		if member.Role == "leader" {
			n.LeaderID = member.PersonID
		}
	}
	changes := []string{}
	if in.Title != nil {
		changes = append(changes, "title")
	}
	if in.CaveSite != nil {
		changes = append(changes, "cave_site")
	}
	if in.TargetDepthM != nil {
		changes = append(changes, "target_depth_m")
	}
	if in.WindowStart != nil {
		changes = append(changes, "window_start")
	}
	if in.WindowEnd != nil {
		changes = append(changes, "window_end")
	}
	if in.Segments != nil {
		changes = append(changes, "segments")
	}
	if in.TeamMembers != nil {
		changes = append(changes, "team_members")
	}
	sort.Strings(changes)
	return &n, changes, nil
}

func planDiff(previous *LifeSupportPlan, in PlanInput) map[string]any {
	d := map[string]any{}
	if previous == nil {
		return d
	}
	if !reflect.DeepEqual(previous.GasMixes, in.GasMixes) {
		d["gas_mixes"] = map[string]any{"before": previous.GasMixes, "after": in.GasMixes}
	}
	if previous.TurnPressureBar != in.TurnPressureBar {
		d["turn_pressure_bar"] = map[string]any{"before": previous.TurnPressureBar, "after": in.TurnPressureBar}
	}
	if previous.ReserveRule != in.ReserveRule {
		d["reserve_rule"] = map[string]any{"before": previous.ReserveRule, "after": in.ReserveRule}
	}
	if !reflect.DeepEqual(previous.SupportAssignments, in.SupportAssignments) {
		d["support_assignments"] = map[string]any{"before": previous.SupportAssignments, "after": in.SupportAssignments}
	}
	if !reflect.DeepEqual(previous.Members, in.Members) {
		d["members"] = map[string]any{"before": previous.Members, "after": in.Members}
	}
	if !reflect.DeepEqual(previous.MemberGasAssignments, in.MemberGasAssignments) {
		d["member_gas_assignments"] = map[string]any{"before": previous.MemberGasAssignments, "after": in.MemberGasAssignments}
	}
	if !reflect.DeepEqual(previous.SegmentGasBudgets, in.SegmentGasBudgets) {
		d["segment_gas_budgets"] = map[string]any{"before": previous.SegmentGasBudgets, "after": in.SegmentGasBudgets}
	}
	return d
}

type RiskDiff struct {
	SegmentName string      `json:"segment_name"`
	Before      SegmentRisk `json:"before"`
	After       SegmentRisk `json:"after"`
}

func RiskDifferences(before, after []SegmentRisk) []RiskDiff {
	bm := map[string]SegmentRisk{}
	for _, r := range before {
		bm[r.SegmentName] = r
	}
	out := make([]RiskDiff, 0, len(after))
	for _, r := range after {
		if b, ok := bm[r.SegmentName]; ok {
			if b.CurrentLevel != r.CurrentLevel || b.VisibilityM != r.VisibilityM || b.RestrictionGrade != r.RestrictionGrade || b.ExitLimitMin != r.ExitLimitMin || b.RiskLevel != r.RiskLevel || b.Score != r.Score || strings.Join(b.Mitigations, "\x00") != strings.Join(r.Mitigations, "\x00") || !reflect.DeepEqual(b.MitigationActions, r.MitigationActions) {
				out = append(out, RiskDiff{r.SegmentName, b, r})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SegmentName < out[j].SegmentName })
	return out
}

func CalculateDepthAdaptation(m *DiveMission, mix GasMix) GasDepthAdaptation {
	depth := mix.IntendedDepthM
	if depth <= 0 {
		depth = m.TargetDepthM
	}
	pressure := 1 + depth/10
	ppo2 := pressure * (mix.OxygenPercent / 100)
	mod := ((1.4 / (mix.OxygenPercent / 100)) - 1) * 10
	if mod < 0 {
		mod = 0
	}
	ead := (depth+10)*(1-mix.HeliumPercent/100) - 10
	if ead < 0 {
		ead = 0
	}
	return GasDepthAdaptation{GasName: mix.Name, IntendedDepthM: depth, OxygenPartialPressureBar: round2(ppo2), MaximumOperatingDepthM: round2(mod), EquivalentNarcoticDepthM: round2(ead), Passed: ppo2 >= 0.16 && ppo2 <= 1.4 && mod+0.01 >= depth && ead <= 40}
}
func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }
