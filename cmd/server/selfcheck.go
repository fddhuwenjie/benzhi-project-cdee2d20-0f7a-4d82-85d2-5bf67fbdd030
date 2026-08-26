package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

type selfCheckClient struct {
	baseURL       string
	client        *http.Client
	missionID     string
	revision      int64
	requestNumber int
}

func runSelfCheck(parent context.Context, cfg config) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	tempDir, err := os.MkdirTemp("", "dive-mission-selfcheck-")
	if err != nil {
		return fmt.Errorf("创建自检临时目录: %w", err)
	}
	defer os.RemoveAll(tempDir)
	cfg.databasePath = filepath.Join(tempDir, "selfcheck.db")
	app, err := buildApplication(ctx, cfg)
	if err != nil {
		return err
	}
	defer app.close()
	listener, err := net.Listen("tcp", cfg.address)
	if err != nil {
		return fmt.Errorf("自检监听 %s: %w", cfg.address, err)
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- app.httpServer.Serve(listener) }()
	client := &selfCheckClient{baseURL: "http://" + listener.Addr().String(), client: &http.Client{Timeout: 5 * time.Second}}
	if err := client.completeWorkflow(ctx); err != nil {
		_ = app.httpServer.Close()
		<-serverErrors
		return err
	}
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := app.httpServer.Shutdown(shutdownContext); err != nil {
		return err
	}
	if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	fmt.Println("自检通过：已通过真实 HTTP 请求完成任务建档、整改复验、签发和不可变归档")
	return nil
}

func (c *selfCheckClient) completeWorkflow(ctx context.Context) error {
	now := time.Now().UTC().Add(-time.Hour)
	create := map[string]any{"title": "地下河取样任务", "cave_site": "白岩洞 A 区", "target_depth_m": 42,
		"window_start": now, "window_end": now.Add(7 * time.Hour), "segments": []string{"入口段", "竖井段"},
		"team_members":          []map[string]any{{"person_id": "leader-1", "name": "领队甲", "role": "leader"}, {"person_id": "support-1", "name": "支援乙", "role": "support"}, {"person_id": "standby-1", "name": "待命丙", "role": "standby"}},
		"member_qualifications": []map[string]any{{"person_id": "leader-1", "qualification_code": "cave_leader", "qualification_level": 2, "valid_until": now.Add(30 * 24 * time.Hour), "evidence_digest": fmt.Sprintf("%064x", 101)}, {"person_id": "support-1", "qualification_code": "cave_support", "qualification_level": 2, "valid_until": now.Add(30 * 24 * time.Hour), "evidence_digest": fmt.Sprintf("%064x", 102)}, {"person_id": "standby-1", "qualification_code": "standby_rescue", "qualification_level": 2, "valid_until": now.Add(30 * 24 * time.Hour), "evidence_digest": fmt.Sprintf("%064x", 103)}}}
	if err := c.write(ctx, "POST", "/api/v1/dive-missions", create, "leader-1"); err != nil {
		return err
	}
	risks := map[string]any{"risks": []map[string]any{
		{"segment_name": "入口段", "current_level": 2, "visibility_m": 8, "restriction_grade": 2, "exit_limit_min": 30, "hazards": []string{"落石"}, "mitigations": []string{"设置观察哨"}},
		{"segment_name": "竖井段", "current_level": 3, "visibility_m": 5, "restriction_grade": 3, "exit_limit_min": 60, "hazards": []string{"狭窄"}, "mitigations": []string{"单人依次通过"}, "mitigation_actions": []map[string]any{{"action_code": "shaft-restriction-1", "hazard": "狭窄", "owner_person_id": "support-1", "due_at": now.Add(-30 * time.Minute), "completion_criteria": "完成全员单人依次通过复核"}}}}}
	if err := c.write(ctx, "POST", c.path("/risks"), risks, "risk-assessor"); err != nil {
		return err
	}
	if err := c.write(ctx, "POST", c.path("/risks/mitigations/shaft-restriction-1/complete"), map[string]any{"result": "已完成狭窄点依次通过复核", "evidence_digest": fmt.Sprintf("%064x", 104)}, "support-1"); err != nil {
		return err
	}
	gasAssignments := []map[string]any{}
	for _, personID := range []string{"leader-1", "support-1", "standby-1"} {
		gasAssignments = append(gasAssignments, map[string]any{"person_id": personID, "surface_consumption_l_min": 1, "primary_asset_id": personID + "-primary", "primary_gas_mix": "bottom-trimix", "redundant_asset_id": personID + "-backup", "redundant_gas_mix": "backup-trimix"})
	}
	sources := map[string][]string{"leader-1": {"primary", "redundant"}, "support-1": {"primary", "redundant"}, "standby-1": {"primary", "redundant"}}
	multipliers := map[string]float64{"leader-1": 1, "support-1": 1, "standby-1": 1}
	plan := map[string]any{"members": create["team_members"], "gas_mixes": []map[string]any{{"name": "bottom-trimix", "oxygen_percent": 21, "helium_percent": 35, "start_pressure_bar": 230, "cylinder_liters": 12, "intended_depth_m": 42}, {"name": "backup-trimix", "oxygen_percent": 21, "helium_percent": 35, "start_pressure_bar": 220, "cylinder_liters": 12, "intended_depth_m": 42}}, "turn_pressure_bar": 140, "reserve_rule": "rule_of_thirds", "support_assignments": []map[string]string{{"person_id": "support-1", "duty": "surface_support"}, {"person_id": "standby-1", "duty": "standby_diver"}}, "member_gas_assignments": gasAssignments, "segment_gas_budgets": []map[string]any{{"segment_name": "入口段", "expected_depth_m": 20, "one_way_minutes": 1, "consumption_multipliers": multipliers, "available_sources": sources}, {"segment_name": "竖井段", "expected_depth_m": 42, "one_way_minutes": 1, "consumption_multipliers": multipliers, "available_sources": sources}}}
	if err := c.write(ctx, "POST", c.path("/life-support-plan"), plan, "leader-1"); err != nil {
		return err
	}
	if err := c.write(ctx, "POST", c.path("/life-support-review"), map[string]any{"decision": "approve", "reason": "方案余量满足要求"}, "reviewer-1"); err != nil {
		return err
	}
	for index, code := range []string{"primary_breathing", "backup_breathing", "primary_lighting", "guideline", "communication"} {
		measurements := map[string]any{}
		switch code {
		case "primary_breathing", "backup_breathing":
			measurements = map[string]any{"start_pressure_bar": map[string]any{"value": 230, "unit": "bar"}, "leak_drop_bar": map[string]any{"value": 2, "unit": "bar"}}
		case "primary_lighting":
			measurements = map[string]any{"runtime_minutes": map[string]any{"value": 180, "unit": "min"}}
		case "guideline":
			measurements = map[string]any{"usable_length_m": map[string]any{"value": 300, "unit": "m"}, "tensile_strength_n": map[string]any{"value": 1000, "unit": "N"}}
		case "communication":
			measurements = map[string]any{"coverage_distance_m": map[string]any{"value": 300, "unit": "m"}, "battery_percent": map[string]any{"value": 90, "unit": "%"}}
		}
		body := map[string]any{"check_code": code, "outcome": "pass", "evidence_digest": fmt.Sprintf("%064x", index+1), "asset_id": fmt.Sprintf("asset-%d", index+1), "inspected_at": time.Now().UTC(), "valid_until": now.Add(7 * time.Hour), "measurements": measurements}
		if err := c.write(ctx, "POST", c.path("/equipment-verifications"), body, "equipment-1"); err != nil {
			return err
		}
	}
	if err := c.write(ctx, "POST", c.path("/drills"), map[string]any{"check_code": "lost_contact", "evidence_digest": fmt.Sprintf("%064x", 20), "conducted_at": time.Now().UTC(), "observed_duration_seconds": 181, "completed_steps": []string{"signal_attempted", "line_search_completed", "team_regrouped"}}, "trainer-1"); err != nil {
		return err
	}
	if err := c.write(ctx, "POST", c.path("/drills"), map[string]any{"check_code": "gas_sharing", "evidence_digest": fmt.Sprintf("%064x", 21), "conducted_at": time.Now().UTC(), "observed_duration_seconds": 60, "completed_steps": []string{"donor_identified", "gas_shared", "controlled_exit_started"}}, "trainer-1"); err != nil {
		return err
	}
	if err := c.write(ctx, "POST", c.path("/remediations"), map[string]any{"check_code": "lost_contact", "corrective_action": "重新统一计时信号并完成桌面推演", "evidence_digest": fmt.Sprintf("%064x", 22), "completed_at": time.Now().UTC(), "delay_reason": "自检使用已开始的任务时间窗验证逾期闭环", "delay_reviewed_by": "safety-reviewer-2"}, "leader-1"); err != nil {
		return err
	}
	if err := c.write(ctx, "POST", c.path("/retests"), map[string]any{"check_code": "lost_contact", "outcome": "pass", "evidence_digest": fmt.Sprintf("%064x", 23)}, "trainer-2"); err != nil {
		return err
	}
	if err := c.write(ctx, "POST", c.path("/release"), map[string]any{"decision": "sign", "reason": "全部门禁满足"}, "supervisor-1"); err != nil {
		return err
	}
	if err := c.write(ctx, "POST", c.path("/archive"), map[string]any{}, "supervisor-1"); err != nil {
		return err
	}
	for _, suffix := range []string{"", "/history", "/audit-events?limit=50", "/archive/evidence?gate_code=equipment&limit=10"} {
		if err := c.read(ctx, c.path(suffix)); err != nil {
			return err
		}
	}
	return nil
}

func (c *selfCheckClient) path(suffix string) string {
	return "/api/v1/dive-missions/" + c.missionID + suffix
}

func (c *selfCheckClient) write(ctx context.Context, method, path string, payload map[string]any, actor string) error {
	c.requestNumber++
	payload["request_id"] = fmt.Sprintf("selfcheck-%03d", c.requestNumber)
	payload["expected_revision"] = c.revision
	payload["actor_id"] = actor
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("自检请求 %s: %w", path, err)
	}
	defer response.Body.Close()
	var result mission.CommandResult
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure any
		_ = json.NewDecoder(response.Body).Decode(&failure)
		return fmt.Errorf("自检请求 %s 返回 %d: %v", path, response.StatusCode, failure)
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return err
	}
	if result.Mission == nil {
		return fmt.Errorf("自检响应缺少 mission")
	}
	c.missionID, c.revision = result.Mission.ID, result.Mission.Revision
	return nil
}

func (c *selfCheckClient) read(ctx context.Context, path string) error {
	request, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("自检查询 %s 返回 %d", path, response.StatusCode)
	}
	return nil
}
