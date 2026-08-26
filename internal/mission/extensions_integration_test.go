package mission_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/store"
)

func createMission(t *testing.T, service *mission.Service, requestID, site string, start time.Time) *mission.DiveMission {
	t.Helper()
	in := mission.CreateInput{Meta: mission.CommandMeta{RequestID: requestID, ActorID: "owner"}, Title: "水文取样", CaveSite: site, TargetDepthM: 42, WindowStart: start, WindowEnd: start.Add(4 * time.Hour), Segments: []string{"入口段"}, TeamMembers: []mission.Member{{PersonID: "leader", Name: "领队", Role: "leader"}, {PersonID: "support", Name: "支援", Role: "support"}, {PersonID: "standby", Name: "待命", Role: "standby"}}}
	result, _, err := service.Create(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	var command mission.CommandResult
	if err := json.Unmarshal(result.Body, &command); err != nil {
		t.Fatal(err)
	}
	return command.Mission
}

func TestDraftRevisionConflictRollbackAndReplay(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := mission.NewService(repository)
	start := time.Now().UTC().Add(24 * time.Hour)
	_ = createMission(t, service, "create-a", "A 洞", start)
	second := createMission(t, service, "create-b", "B 洞", start)

	conflictingSite := "A 洞"
	_, _, err = service.ReviseDraft(ctx, second.ID, mission.DraftRevisionInput{Meta: mission.CommandMeta{RequestID: "revise-conflict", ActorID: "owner", ExpectedRevision: second.Revision}, CaveSite: &conflictingSite})
	if got := mission.AsError(err).Code; got != "schedule_conflict" {
		t.Fatalf("冲突错误 = %s", got)
	}
	unchanged, err := service.Mission(ctx, second.ID)
	if err != nil || unchanged.CaveSite != "B 洞" || unchanged.Revision != 1 {
		t.Fatalf("冲突后任务发生变化: %+v, %v", unchanged, err)
	}

	segments := []string{"竖井段", "水道段"}
	members := []mission.Member{{PersonID: "leader-2", Name: "新领队", Role: "leader"}, {PersonID: "support", Name: "支援", Role: "support"}, {PersonID: "standby", Name: "待命", Role: "standby"}}
	in := mission.DraftRevisionInput{Meta: mission.CommandMeta{RequestID: "revise-ok", ActorID: "owner", ExpectedRevision: 1}, Segments: &segments, TeamMembers: &members}
	_, replay, err := service.ReviseDraft(ctx, second.ID, in)
	if err != nil || replay {
		t.Fatalf("首次修订失败: replay=%t err=%v", replay, err)
	}
	_, replay, err = service.ReviseDraft(ctx, second.ID, in)
	if err != nil || !replay {
		t.Fatalf("幂等重放失败: replay=%t err=%v", replay, err)
	}
	updated, err := service.Mission(ctx, second.ID)
	if err != nil || updated.Revision != 2 || updated.LeaderID != "leader-2" || len(updated.Segments) != 2 || len(updated.TeamMembers) != 3 {
		t.Fatalf("修订聚合不正确: %+v, %v", updated, err)
	}
}

func TestPlanDepthAdaptationGate(t *testing.T) {
	m := &mission.DiveMission{TargetDepthM: 42, TeamMembers: []mission.Member{{PersonID: "leader", Role: "leader"}, {PersonID: "support", Role: "support"}, {PersonID: "standby", Role: "standby"}}}
	base := mission.PlanInput{Members: m.TeamMembers, TurnPressureBar: 140, ReserveRule: "rule_of_thirds", SupportAssignments: []mission.SupportAssignment{{PersonID: "support", Duty: "surface_support"}, {PersonID: "standby", Duty: "standby_diver"}}}
	base.GasMixes = []mission.GasMix{{Name: "air", OxygenPercent: 32, StartPressureBar: 230, CylinderLiters: 12, IntendedDepthM: 42}, {Name: "backup", OxygenPercent: 21, HeliumPercent: 35, StartPressureBar: 230, CylinderLiters: 12, IntendedDepthM: 42}}
	if _, err := mission.ValidatePlan(m, base); mission.AsError(err).Status != 422 {
		t.Fatalf("高氧主用气体应被拒绝: %v", err)
	}
	base.GasMixes[0] = mission.GasMix{Name: "trimix", OxygenPercent: 21, HeliumPercent: 35, StartPressureBar: 230, CylinderLiters: 12, IntendedDepthM: 42}
	if _, err := mission.ValidatePlan(m, base); err != nil {
		t.Fatalf("合规气体被拒绝: %v", err)
	}
}
