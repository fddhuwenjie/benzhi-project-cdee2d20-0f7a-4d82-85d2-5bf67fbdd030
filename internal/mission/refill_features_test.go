package mission_test

import (
	"context"
	"testing"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/store"
)

func newFeatureService(t *testing.T) (*store.Store, *mission.Service) {
	t.Helper()
	repository, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return repository, mission.NewService(repository)
}

func TestSchedulePreflightIsReadOnlyAndStaleDigestConflicts(t *testing.T) {
	ctx := context.Background()
	repository, service := newFeatureService(t)
	defer repository.Close()
	start := time.Now().UTC().Add(2 * time.Hour)
	preflight, err := service.PreflightSchedule(ctx, "preflight-1", "  白岩洞  A区 ", start, start.Add(2*time.Hour))
	if err != nil || !preflight.ReadOnly || !preflight.Available || preflight.CaveSiteKey != "白岩洞 a区" || preflight.SourceDigest == "" {
		t.Fatalf("预检结果不正确: %+v, %v", preflight, err)
	}
	listed, err := service.List(ctx, mission.ListFilter{Limit: 10})
	if err != nil || len(listed.Items) != 0 {
		t.Fatalf("预检产生了任务写入: %+v, %v", listed, err)
	}
	_ = createMission(t, service, "occupy-window", "白岩洞 A区", start.Add(time.Hour))
	in := mission.CreateInput{Meta: mission.CommandMeta{RequestID: "stale-create", ActorID: "owner"}, Title: "竞争创建", CaveSite: "白岩洞 A区", TargetDepthM: 20, WindowStart: start, WindowEnd: start.Add(2 * time.Hour), Segments: []string{"入口"}, TeamMembers: []mission.Member{{PersonID: "leader", Name: "甲", Role: "leader"}, {PersonID: "support", Name: "乙", Role: "support"}, {PersonID: "standby", Name: "丙", Role: "standby"}}, SourceDigest: preflight.SourceDigest}
	if _, _, err := service.Create(ctx, in); mission.AsError(err).Code != "schedule_conflict" {
		t.Fatalf("过期预检摘要未阻止创建: %v", err)
	}
	listed, _ = service.List(ctx, mission.ListFilter{Limit: 10})
	if len(listed.Items) != 1 {
		t.Fatalf("冲突创建留下了半成品: %+v", listed.Items)
	}
}

func TestRiskFiltersAndStatisticsUseFullResult(t *testing.T) {
	ctx := context.Background()
	repository, service := newFeatureService(t)
	defer repository.Close()
	start := time.Now().UTC().Add(time.Hour)
	m := createMission(t, service, "risk-create", "风险洞", start)
	risks := []mission.SegmentRisk{{SegmentName: "入口段", CurrentLevel: 5, VisibilityM: 1, RestrictionGrade: 5, ExitLimitMin: 120, Hazards: []string{"急流"}, Mitigations: []string{"固定导向", "设置支援"}}}
	if _, _, err := service.SubmitRisks(ctx, m.ID, mission.RiskInput{Meta: mission.CommandMeta{RequestID: "risk-submit", ActorID: "assessor", ExpectedRevision: 1}, Risks: risks}); err != nil {
		t.Fatal(err)
	}
	result, err := service.List(ctx, mission.ListFilter{RiskLevel: "critical", MinTotalScore: 10, MitigationState: "complete", Limit: 1})
	if err != nil || len(result.Items) != 1 || result.Statistics.RiskHighest["critical"] != 1 || result.Statistics.MitigationStates["complete"] != 1 {
		t.Fatalf("风险筛选或完整统计不正确: %+v, %v", result, err)
	}
}

func advanceToDrillPending(t *testing.T, service *mission.Service) *mission.DiveMission {
	t.Helper()
	ctx := context.Background()
	start := time.Now().UTC().Add(-time.Hour)
	m := createMission(t, service, "drill-create", "演练洞", start)
	risks := []mission.SegmentRisk{{SegmentName: "入口段", CurrentLevel: 2, VisibilityM: 8, RestrictionGrade: 2, ExitLimitMin: 30, Hazards: []string{"落石"}, Mitigations: []string{"观察哨"}}}
	if _, _, err := service.SubmitRisks(ctx, m.ID, mission.RiskInput{Meta: mission.CommandMeta{RequestID: "drill-risks", ActorID: "assessor", ExpectedRevision: 1}, Risks: risks}); err != nil {
		t.Fatal(err)
	}
	plan := mission.PlanInput{Meta: mission.CommandMeta{RequestID: "drill-plan", ActorID: "leader", ExpectedRevision: 2}, Members: m.TeamMembers, GasMixes: []mission.GasMix{{Name: "bottom", OxygenPercent: 21, HeliumPercent: 35, StartPressureBar: 230, CylinderLiters: 12, IntendedDepthM: 42}, {Name: "backup", OxygenPercent: 21, HeliumPercent: 35, StartPressureBar: 220, CylinderLiters: 12, IntendedDepthM: 42}}, TurnPressureBar: 140, ReserveRule: "rule_of_thirds", SupportAssignments: []mission.SupportAssignment{{PersonID: "support", Duty: "surface_support"}, {PersonID: "standby", Duty: "standby_diver"}}}
	if _, _, err := service.SubmitPlan(ctx, m.ID, plan); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ReviewPlan(ctx, m.ID, mission.ReviewInput{Meta: mission.CommandMeta{RequestID: "drill-review", ActorID: "reviewer", ExpectedRevision: 3}, Decision: "approve"}); err != nil {
		t.Fatal(err)
	}
	items := make([]mission.EquipmentInput, 0, 5)
	for i, code := range []string{"primary_breathing", "backup_breathing", "primary_lighting", "guideline", "communication"} {
		items = append(items, mission.EquipmentInput{CheckCode: code, Outcome: "pass", EvidenceDigest: digest(i + 1), AssetID: code + "-asset", InspectedAt: time.Now().UTC(), ValidUntil: start.Add(5 * time.Hour)})
	}
	if _, _, err := service.VerifyEquipmentBatch(ctx, m.ID, mission.EquipmentBatchInput{Meta: mission.CommandMeta{RequestID: "drill-equipment", ActorID: "equipment", ExpectedRevision: 4}, Items: items}); err != nil {
		t.Fatal(err)
	}
	current, err := service.Mission(ctx, m.ID)
	if err != nil || current.Status != mission.StatusDrillPending {
		t.Fatalf("未进入演练待办: %+v, %v", current, err)
	}
	return current
}

func digest(n int) string {
	const hex = "0123456789abcdef"
	result := make([]byte, 64)
	for i := range result {
		result[i] = hex[(n+i)%len(hex)]
	}
	return string(result)
}

func TestDrillAndRemediationBatchesAreAtomic(t *testing.T) {
	ctx := context.Background()
	repository, service := newFeatureService(t)
	defer repository.Close()
	m := advanceToDrillPending(t, service)
	conducted := time.Now().UTC()
	duplicate := digest(20)
	badDrills := mission.DrillBatchInput{Meta: mission.CommandMeta{RequestID: "bad-drills", ActorID: "witness", ExpectedRevision: m.Revision}, Items: []mission.DrillInput{{CheckCode: "lost_contact", Outcome: "deviation", Deviation: "超时", EvidenceDigest: duplicate, ConductedAt: conducted, DurationSeconds: 60}, {CheckCode: "gas_sharing", Outcome: "pass", EvidenceDigest: duplicate, ConductedAt: conducted, DurationSeconds: 60}}}
	if _, _, err := service.RecordDrillBatch(ctx, m.ID, badDrills); mission.AsError(err).Status != 422 {
		t.Fatalf("重复摘要未拒绝: %v", err)
	}
	unchanged, _ := service.Mission(ctx, m.ID)
	if unchanged.Revision != m.Revision || len(unchanged.Verifications) != 5 {
		t.Fatalf("失败演练批次产生部分写入: %+v", unchanged)
	}
	good := badDrills
	good.Meta.RequestID = "good-drills"
	good.Items[1].EvidenceDigest = digest(21)
	if _, _, err := service.RecordDrillBatch(ctx, m.ID, good); err != nil {
		t.Fatal(err)
	}
	current, _ := service.Mission(ctx, m.ID)
	badFixes := mission.RemediationBatchInput{Meta: mission.CommandMeta{RequestID: "bad-fixes", ActorID: "fixer", ExpectedRevision: current.Revision}, Items: []mission.RemediationInput{{CheckCode: "lost_contact", CorrectiveAction: "", EvidenceDigest: digest(22), CompletedAt: time.Now().UTC()}}}
	if _, _, err := service.RecordRemediationBatch(ctx, m.ID, badFixes); mission.AsError(err).Status != 422 {
		t.Fatalf("空整改说明未拒绝: %v", err)
	}
	after, _ := service.Mission(ctx, m.ID)
	if after.Revision != current.Revision || len(after.Verifications) != len(current.Verifications) {
		t.Fatalf("失败整改批次产生写入: %+v", after)
	}
}
