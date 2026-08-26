package mission_test

import (
	"context"
	"testing"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func TestRiskPreviewReadOnlyAndLockedToRevision(t *testing.T) {
	ctx := context.Background()
	repository, service := newFeatureService(t)
	defer repository.Close()
	m := createMission(t, service, "preview-create", "试算洞", time.Now().UTC().Add(time.Hour))
	risks := []mission.SegmentRisk{{SegmentName: "入口段", CurrentLevel: 2, VisibilityM: 8, RestrictionGrade: 2, ExitLimitMin: 30, Hazards: []string{"落石"}, Mitigations: []string{"观察哨"}}}
	preview, err := service.PreviewRisks(ctx, m.ID, risks)
	if err != nil || !preview.Passed || preview.SourceRevision != 1 || preview.RiskPreviewDigest == "" {
		t.Fatalf("风险试算不正确: %+v, %v", preview, err)
	}
	events, _ := repository.AllEvents(ctx, m.ID)
	if len(events) != 1 {
		t.Fatalf("试算写入了审计事件: %+v", events)
	}
	title := "修订后的任务"
	if _, _, err := service.ReviseDraft(ctx, m.ID, mission.DraftRevisionInput{Meta: mission.CommandMeta{RequestID: "preview-revise", ActorID: "owner", ExpectedRevision: 1}, Title: &title}); err != nil {
		t.Fatal(err)
	}
	_, _, err = service.SubmitRisks(ctx, m.ID, mission.RiskInput{Meta: mission.CommandMeta{RequestID: "preview-submit", ActorID: "assessor", ExpectedRevision: 2}, Risks: risks, RiskPreviewDigest: preview.RiskPreviewDigest})
	if mission.AsError(err).Code != "preview_conflict" {
		t.Fatalf("旧试算摘要未冲突: %v", err)
	}
	current, _ := service.Mission(ctx, m.ID)
	if current.Status != mission.StatusDraft || current.Revision != 2 || len(current.Risks) != 0 {
		t.Fatalf("冲突提交产生了写入: %+v", current)
	}
}

func TestMemberGasAssignmentsRequireIndependentCapacity(t *testing.T) {
	m := &mission.DiveMission{TargetDepthM: 20, TeamMembers: []mission.Member{{PersonID: "leader", Role: "leader"}, {PersonID: "support", Role: "support"}, {PersonID: "standby", Role: "standby"}}, Risks: []mission.SegmentRisk{{ExitLimitMin: 10}}}
	mixes := []mission.GasMix{{Name: "primary", OxygenPercent: 21, StartPressureBar: 300, CylinderLiters: 20, IntendedDepthM: 20}, {Name: "backup", OxygenPercent: 21, StartPressureBar: 300, CylinderLiters: 20, IntendedDepthM: 20}}
	assignments := []mission.MemberGasAssignment{
		{PersonID: "leader", SurfaceConsumptionLMin: 10, PrimaryAssetID: "l-p", PrimaryGasMix: "primary", RedundantAssetID: "l-r", RedundantGasMix: "backup"},
		{PersonID: "support", SurfaceConsumptionLMin: 10, PrimaryAssetID: "s-p", PrimaryGasMix: "primary", RedundantAssetID: "s-r", RedundantGasMix: "backup"},
		{PersonID: "standby", SurfaceConsumptionLMin: 10, PrimaryAssetID: "b-p", PrimaryGasMix: "primary", RedundantAssetID: "b-r", RedundantGasMix: "backup"},
	}
	margins, err := mission.CalculateMemberGasMargins(m, assignments, mixes, 100)
	if err != nil || len(margins) != 6 {
		t.Fatalf("合规成员气体分配未通过: %+v, %v", margins, err)
	}
	assignments[2].RedundantAssetID = "b-p"
	if _, err := mission.CalculateMemberGasMargins(m, assignments, mixes, 100); mission.AsError(err).Status != 422 {
		t.Fatalf("主备复用资产未被拒绝: %v", err)
	}
}
