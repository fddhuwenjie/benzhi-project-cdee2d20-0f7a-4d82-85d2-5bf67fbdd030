package mission

import (
	"testing"
	"time"
)

func validCreateInput() CreateInput {
	start := time.Now().UTC().Add(time.Hour)
	return CreateInput{Meta: CommandMeta{RequestID: "request-1", ExpectedRevision: 0, ActorID: "leader-1"}, Title: "科研取样", CaveSite: "测试洞穴", TargetDepthM: 35, WindowStart: start, WindowEnd: start.Add(4 * time.Hour), Segments: []string{"入口", "深水段"}, TeamMembers: []Member{{PersonID: "leader-1", Name: "甲", Role: "leader"}, {PersonID: "support-1", Name: "乙", Role: "support"}, {PersonID: "standby-1", Name: "丙", Role: "standby"}}}
}

func TestValidateCreateRejectsDuplicateRoles(t *testing.T) {
	input := validCreateInput()
	input.TeamMembers[2].Role = "support"
	if err := ValidateCreate(input, time.Now()); err == nil {
		t.Fatal("重复职责未被拒绝")
	}
}

func TestValidateRisksCalculatesStableRiskLevel(t *testing.T) {
	create := validCreateInput()
	m := &DiveMission{Segments: create.Segments}
	risks, err := ValidateRisks(m, RiskInput{Risks: []SegmentRisk{{SegmentName: "深水段", CurrentLevel: 4, VisibilityM: 2, RestrictionGrade: 3, ExitLimitMin: 100, Hazards: []string{"低能见度"}, Mitigations: []string{"双导向", "缩短时间"}}, {SegmentName: "入口", CurrentLevel: 1, VisibilityM: 10, RestrictionGrade: 1, ExitLimitMin: 20, Hazards: []string{"落石"}, Mitigations: []string{"观察"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if risks[0].SegmentName != "入口" || risks[1].RiskLevel != "critical" {
		t.Fatalf("风险排序或等级错误: %#v", risks)
	}
}

func TestAllowedActionsForArchivedMissionIsEmpty(t *testing.T) {
	if actions := AllowedActions(&DiveMission{Status: StatusArchived}); len(actions) != 0 {
		t.Fatalf("归档任务仍有动作: %v", actions)
	}
}

func TestValidatePlanRejectsDuplicateGasWithNameDetail(t *testing.T) {
	create := validCreateInput()
	m := &DiveMission{TargetDepthM: create.TargetDepthM, TeamMembers: create.TeamMembers}
	input := PlanInput{Members: create.TeamMembers, GasMixes: []GasMix{{Name: "air", OxygenPercent: 21, StartPressureBar: 230, CylinderLiters: 12}, {Name: " AIR ", OxygenPercent: 21, StartPressureBar: 230, CylinderLiters: 12}}, TurnPressureBar: 140, ReserveRule: "rule_of_thirds", SupportAssignments: []SupportAssignment{{PersonID: "support-1", Duty: "surface_support"}, {PersonID: "standby-1", Duty: "standby_diver"}}}
	_, err := ValidatePlan(m, input)
	if AsError(err).Status != 422 || AsError(err).Details == nil {
		t.Fatalf("重复气体未返回结构化 422: %v", err)
	}
}
