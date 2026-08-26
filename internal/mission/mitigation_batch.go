package mission

import (
	"context"
	"sort"
	"strings"
)

func (s *Service) CompleteMitigationBatch(ctx context.Context, id string, in MitigationBatchInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "complete_risk_mitigation_batch", in.Meta, in, func(_ context.Context, _ Tx, m *DiveMission) (string, any, error) {
		if m.Status != StatusRiskAssessed {
			return "", nil, Unprocessable("status", "任务必须处于 risk_assessed 状态", map[string]any{"current_status": m.Status})
		}
		if len(in.Items) == 0 {
			return "", nil, Invalid("items", "至少提交一项行动")
		}
		seenCodes, seenEvidence := map[string]bool{}, map[string]bool{}
		for _, r := range append(append([]SegmentRisk(nil), m.RiskHistory...), m.Risks...) {
			for _, a := range r.MitigationActions {
				if a.EvidenceDigest != "" {
					seenEvidence[strings.ToLower(a.EvidenceDigest)] = true
				}
			}
		}
		type target struct{ risk, action int }
		targets := make([]target, 0, len(in.Items))
		now := s.now().UTC()
		for _, item := range in.Items {
			code := strings.ToLower(strings.TrimSpace(item.ActionCode))
			if code == "" {
				return "", nil, Unprocessable("action_code", "不能为空", nil)
			}
			if seenCodes[code] {
				return "", nil, Unprocessable("action_code", "批次内行动代码必须唯一", map[string]any{"action_code": code})
			}
			seenCodes[code] = true
			if strings.TrimSpace(item.Result) == "" {
				return "", nil, Unprocessable("result", "必须提交结果说明", map[string]any{"action_code": code})
			}
			digest := strings.ToLower(strings.TrimSpace(item.EvidenceDigest))
			if !validEvidence(digest) {
				return "", nil, Unprocessable("evidence_digest", "必须为 16 到 128 位十六进制摘要", map[string]any{"action_code": code})
			}
			if seenEvidence[digest] {
				return "", nil, Unprocessable("evidence_digest", "缓解证据摘要必须全任务唯一", map[string]any{"action_code": code})
			}
			seenEvidence[digest] = true
			completedAt := item.CompletedAt
			if completedAt.IsZero() {
				completedAt = now
			} else {
				completedAt = completedAt.UTC()
			}
			if completedAt.After(now) {
				return "", nil, Unprocessable("completed_at", "完成时间不能晚于当前时间", map[string]any{"action_code": code})
			}
			found := false
			for ri := range m.Risks {
				for ai := range m.Risks[ri].MitigationActions {
					a := m.Risks[ri].MitigationActions[ai]
					if a.Code != code {
						continue
					}
					found = true
					if a.OwnerPersonID != in.Meta.ActorID {
						return "", nil, Unprocessable("actor_id", "缓解行动只能由登记负责人完成", map[string]any{"action_code": code, "owner_person_id": a.OwnerPersonID})
					}
					if a.Status == "completed" {
						return "", nil, Unprocessable("action_code", "已完成行动不可覆盖", map[string]any{"action_code": code})
					}
					targets = append(targets, target{ri, ai})
				}
			}
			if !found {
				return "", nil, Unprocessable("action_code", "行动不属于当前风险版本", map[string]any{"action_code": code})
			}
		}
		completed := make([]MitigationAction, 0, len(in.Items))
		for i, item := range in.Items {
			t := targets[i]
			a := &m.Risks[t.risk].MitigationActions[t.action]
			at := item.CompletedAt.UTC()
			if item.CompletedAt.IsZero() {
				at = now
			}
			a.Status, a.Result, a.EvidenceDigest, a.CompletedAt = "completed", strings.TrimSpace(item.Result), strings.ToLower(strings.TrimSpace(item.EvidenceDigest)), &at
			completed = append(completed, *a)
		}
		sort.Slice(completed, func(i, j int) bool { return completed[i].Code < completed[j].Code })
		remaining := mitigationBlockers(m, now)
		return "risk_mitigations_batch_completed", map[string]any{"completed_items": completed, "remaining_blockers": remaining, "allowed_actions": AllowedActions(m), "life_support_plan_allowed": len(remaining) == 0}, nil
	})
}
