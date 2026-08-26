package mission_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func TestMitigationBatchIsAtomicAndReplayable(t *testing.T) {
	ctx := context.Background()
	repository, service := newFeatureService(t)
	defer repository.Close()
	m := createMission(t, service, "batch-create", "批量洞", time.Now().UTC().Add(4*time.Hour))
	due := m.WindowStart.Add(-time.Hour)
	risks := []mission.SegmentRisk{{
		SegmentName: "入口段", CurrentLevel: 5, VisibilityM: 1, RestrictionGrade: 5, ExitLimitMin: 120,
		Hazards: []string{"急流", "落石", "迷航"}, Mitigations: []string{"固定导向", "设置支援"},
		MitigationActions: []mission.MitigationAction{
			{Code: "flow", Hazard: "急流", OwnerPersonID: "support", DueAt: due, CompletionCriteria: "完成急流锚点复核并记录"},
			{Code: "rock", Hazard: "落石", OwnerPersonID: "support", DueAt: due, CompletionCriteria: "完成落石区域清理并记录"},
			{Code: "route", Hazard: "迷航", OwnerPersonID: "support", DueAt: due, CompletionCriteria: "完成全程导向复核并记录"},
		},
	}}
	if _, _, err := service.SubmitRisks(ctx, m.ID, mission.RiskInput{Meta: mission.CommandMeta{RequestID: "batch-risks", ActorID: "assessor", ExpectedRevision: 1}, Risks: risks}); err != nil {
		t.Fatal(err)
	}
	bad := mission.MitigationBatchInput{Meta: mission.CommandMeta{RequestID: "batch-bad", ActorID: "support", ExpectedRevision: 2}, Items: []mission.MitigationBatchItem{
		{ActionCode: "flow", Result: "完成", EvidenceDigest: digest(30)},
		{ActionCode: "rock", Result: "完成", EvidenceDigest: digest(31)},
		{ActionCode: "route", Result: "完成", EvidenceDigest: digest(30)},
	}}
	if _, _, err := service.CompleteMitigationBatch(ctx, m.ID, bad); mission.AsError(err).Status != 422 {
		t.Fatalf("重复证据未整批拒绝: %v", err)
	}
	current, _ := service.Mission(ctx, m.ID)
	if current.Revision != 2 || mission.RiskMitigationState(current) != "incomplete" {
		t.Fatalf("失败批次产生了部分写入: %+v", current)
	}
	good := bad
	good.Meta.RequestID = "batch-good"
	good.Items[2].EvidenceDigest = digest(32)
	first, replay, err := service.CompleteMitigationBatch(ctx, m.ID, good)
	if err != nil || replay {
		t.Fatalf("成功批次失败: %v", err)
	}
	second, replay, err := service.CompleteMitigationBatch(ctx, m.ID, good)
	if err != nil || !replay || string(first.Body) != string(second.Body) {
		t.Fatalf("幂等回放不一致: %v", err)
	}
	var payload struct {
		Mission   mission.DiveMission        `json:"mission"`
		Completed []mission.MitigationAction `json:"completed_items"`
		Allowed   []string                   `json:"allowed_actions"`
	}
	if err := json.Unmarshal(first.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Mission.Revision != 3 || len(payload.Completed) != 3 || len(payload.Allowed) < 1 || payload.Allowed[0] != "reassess_risks" && payload.Allowed[0] != "submit_life_support_plan" {
		t.Fatalf("批次响应不正确: %+v", payload)
	}
	events, _ := repository.AllEvents(ctx, m.ID)
	if len(events) != 3 || events[2].EventType != "risk_mitigations_batch_completed" {
		t.Fatalf("批次审计事件不正确: %+v", events)
	}
}
