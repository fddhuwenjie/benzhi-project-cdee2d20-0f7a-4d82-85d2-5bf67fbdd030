package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func TestCreatePersistsAndReplaysIdempotently(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "missions.db")
	repository, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	service := mission.NewService(repository)
	start := time.Now().UTC().Add(time.Hour)
	input := mission.CreateInput{Meta: mission.CommandMeta{RequestID: "create-1", ActorID: "leader-1"}, Title: "持久化测试", CaveSite: "测试洞穴", TargetDepthM: 20, WindowStart: start, WindowEnd: start.Add(time.Hour), Segments: []string{"入口"}, TeamMembers: []mission.Member{{PersonID: "leader-1", Name: "甲", Role: "leader"}, {PersonID: "support-1", Name: "乙", Role: "support"}, {PersonID: "standby-1", Name: "丙", Role: "standby"}}}
	first, replay, err := service.Create(ctx, input)
	if err != nil || replay {
		t.Fatalf("首次创建失败: replay=%v err=%v", replay, err)
	}
	second, replay, err := service.Create(ctx, input)
	if err != nil || !replay {
		t.Fatalf("重复请求未重放: replay=%v err=%v", replay, err)
	}
	if string(first.Body) != string(second.Body) {
		t.Fatal("幂等重放响应不一致")
	}
	var response mission.CommandResult
	if err := json.Unmarshal(first.Body, &response); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	loaded, err := repository.Mission(ctx, response.Mission.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 1 || loaded.Status != mission.StatusDraft || len(loaded.TeamMembers) != 3 {
		t.Fatalf("重开后任务不完整: %#v", loaded)
	}
}

func TestRevisionConflictRollsBackCommandResult(t *testing.T) {
	ctx := context.Background()
	repository, err := Open(ctx, filepath.Join(t.TempDir(), "conflict.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := mission.NewService(repository)
	start := time.Now().UTC().Add(time.Hour)
	create := mission.CreateInput{Meta: mission.CommandMeta{RequestID: "c1", ActorID: "leader"}, Title: "冲突测试", CaveSite: "洞穴", TargetDepthM: 10, WindowStart: start, WindowEnd: start.Add(time.Hour), Segments: []string{"入口"}, TeamMembers: []mission.Member{{PersonID: "leader", Name: "甲", Role: "leader"}, {PersonID: "support", Name: "乙", Role: "support"}, {PersonID: "standby", Name: "丙", Role: "standby"}}}
	stored, _, err := service.Create(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	var created mission.CommandResult
	if err = json.Unmarshal(stored.Body, &created); err != nil {
		t.Fatal(err)
	}
	risk := mission.RiskInput{Meta: mission.CommandMeta{RequestID: "r1", ExpectedRevision: 0, ActorID: "assessor"}, Risks: []mission.SegmentRisk{{SegmentName: "入口", CurrentLevel: 1, VisibilityM: 10, RestrictionGrade: 1, ExitLimitMin: 20, Hazards: []string{"落石"}, Mitigations: []string{"观察"}}}}
	if _, _, err = service.SubmitRisks(ctx, created.Mission.ID, risk); mission.AsError(err).Code != "revision_conflict" {
		t.Fatalf("未返回修订冲突: %v", err)
	}
	risk.Meta.ExpectedRevision = 1
	if _, replay, err := service.SubmitRisks(ctx, created.Mission.ID, risk); err != nil || replay {
		t.Fatalf("失败请求错误地占用了 request_id: replay=%v err=%v", replay, err)
	}
}

func TestScheduleConflictAndPayloadFingerprintAreAtomic(t *testing.T) {
	ctx := context.Background()
	repository, err := Open(ctx, filepath.Join(t.TempDir(), "schedule.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := mission.NewService(repository)
	zone := time.FixedZone("UTC+8", 8*60*60)
	start := time.Date(2030, 5, 1, 10, 0, 0, 0, zone)
	base := mission.CreateInput{Meta: mission.CommandMeta{RequestID: "schedule-1", ActorID: "leader"}, Title: "首个任务", CaveSite: "  白岩洞 A 区 ", TargetDepthM: 20, WindowStart: start, WindowEnd: start.Add(2 * time.Hour), Segments: []string{"入口"}, TeamMembers: []mission.Member{{PersonID: "leader", Name: "甲", Role: "leader"}, {PersonID: "support", Name: "乙", Role: "support"}, {PersonID: "standby", Name: "丙", Role: "standby"}}}
	stored, _, err := service.Create(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	var created mission.CommandResult
	if err := json.Unmarshal(stored.Body, &created); err != nil {
		t.Fatal(err)
	}
	if created.Mission.WindowStart.Location() != time.UTC || !created.Mission.ScheduleCheck.Checked {
		t.Fatalf("时间或排期结果未规范化: %#v", created.Mission)
	}
	tampered := base
	tampered.Title = "篡改标题"
	if _, _, err := service.Create(ctx, tampered); mission.AsError(err).Code != "idempotency_key_reused" {
		t.Fatalf("未识别载荷指纹变更: %v", err)
	}
	overlap := base
	overlap.Meta.RequestID = "schedule-2"
	overlap.WindowStart = start.Add(90 * time.Minute)
	overlap.WindowEnd = start.Add(3 * time.Hour)
	if _, _, err := service.Create(ctx, overlap); mission.AsError(err).Code != "schedule_conflict" {
		t.Fatalf("未识别站点时间窗冲突: %v", err)
	}
	events, err := repository.AllAuditEvents(ctx, created.Mission.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("失败创建产生了审计写入: len=%d err=%v", len(events), err)
	}
}
