package mission

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)

const drillRuleVersion = "emergency-drill-v1"

var drillRequiredSteps = map[string][]string{
	"lost_contact": {"signal_attempted", "line_search_completed", "team_regrouped"},
	"gas_sharing":  {"donor_identified", "gas_shared", "controlled_exit_started"},
}

func drillDurationLimit(code, riskLevel string) int {
	base := map[string]int{"lost_contact": 180, "gas_sharing": 120}[code]
	if riskLevel == "high" {
		base = base * 3 / 4
	}
	if riskLevel == "critical" {
		base /= 2
	}
	return base
}

func replaceRecord(records []VerificationRecord, replacement VerificationRecord) []VerificationRecord {
	result := make([]VerificationRecord, 0, len(records)+1)
	for _, record := range records {
		if record.RecordType == replacement.RecordType && record.CheckCode == replacement.CheckCode {
			continue
		}
		result = append(result, record)
	}
	return append(result, replacement)
}

func (s *Service) VerifyEquipment(ctx context.Context, id string, in EquipmentInput) (StoredResult, bool, error) {
	return s.VerifyEquipmentBatch(ctx, id, EquipmentBatchInput{Meta: in.Meta, Items: []EquipmentInput{in}})
}

func (s *Service) VerifyEquipmentBatch(ctx context.Context, id string, in EquipmentBatchInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "verify_equipment", in.Meta, in, func(_ context.Context, _ Tx, m *DiveMission) (string, any, error) {
		refreshMode := m.Status == StatusDrillPending || m.Status == StatusReadyForRelease || m.Status == StatusReleaseRejected
		if m.Status != StatusEquipmentVerification && !refreshMode {
			return "", nil, InvalidState(m.Status, "装备核验")
		}
		if in.Meta.ActorID == m.LeaderID || (m.LifeSupportPlan != nil && in.Meta.ActorID == m.LifeSupportPlan.ReviewedBy) {
			return "", nil, Invalid("actor_id", "装备核验员不能是领队或方案审核员")
		}
		if len(in.Items) == 0 {
			return "", nil, Invalid("items", "至少包含一个装备核验项")
		}
		codes, digests, assets := map[string]bool{}, map[string]string{}, map[string]string{}
		failedAssets := map[string]VerificationRecord{}
		for _, record := range latestRecords(m.Verifications, "equipment") {
			digests[record.EvidenceDigest] = record.CheckCode
			if record.AssetID != "" {
				assets[record.AssetID] = record.CheckCode
			}
			if record.Outcome == "fail" {
				failedAssets[record.AssetID] = record
			}
		}
		for _, record := range m.VerificationHistory {
			if record.RecordType != "equipment" {
				continue
			}
			if record.AssetID != "" {
				assets[record.AssetID] = record.CheckCode
			}
			if record.EvidenceDigest != "" {
				digests[record.EvidenceDigest] = record.CheckCode
			}
			if record.Outcome == "fail" {
				failedAssets[record.AssetID] = record
			}
		}
		var records []VerificationRecord
		expiredBefore := expiredEquipmentCodesAt(m, s.now().UTC())
		expiredSet := map[string]bool{}
		for _, code := range expiredBefore {
			expiredSet[code] = true
		}
		for _, item := range in.Items {
			if _, ok := equipmentLabels[item.CheckCode]; !ok {
				return "", nil, Unprocessable("check_code", "不是必检装备代码", map[string]any{"check_code": item.CheckCode})
			}
			if codes[item.CheckCode] {
				return "", nil, Unprocessable("items", "批次内 check_code 必须唯一", map[string]any{"check_code": item.CheckCode})
			}
			codes[item.CheckCode] = true
			if refreshMode && !expiredSet[item.CheckCode] {
				return "", nil, Unprocessable("items", "回补批次只能包含已失效的装备检查项", map[string]any{"check_code": item.CheckCode, "expired": expiredBefore})
			}
			if item.Outcome != "" && item.Outcome != "pass" && item.Outcome != "fail" {
				return "", nil, Unprocessable("outcome", "必须为 pass 或 fail", map[string]any{"check_code": item.CheckCode})
			}
			if item.Outcome == "fail" && len(item.Measurements) == 0 && strings.TrimSpace(item.FailureReason) == "" {
				return "", nil, Unprocessable("failure_reason", "失败项必须提供失败原因", map[string]any{"check_code": item.CheckCode})
			}
			if !validEvidence(item.EvidenceDigest) {
				return "", nil, Unprocessable("evidence_digest", "必须为 16 到 128 位十六进制摘要", map[string]any{"check_code": item.CheckCode})
			}
			assetID := strings.ToLower(strings.Join(strings.Fields(item.AssetID), "-"))
			if assetID == "" {
				return "", nil, Unprocessable("asset_id", "必须绑定具体装备身份", map[string]any{"check_code": item.CheckCode})
			}
			if code, used := assets[assetID]; used && (code != item.CheckCode || failedAssets[assetID].ID != "") {
				return "", nil, Unprocessable("asset_id", "装备已绑定到其他检查项", map[string]any{"check_code": item.CheckCode, "bound_check_code": code, "asset_id": assetID})
			}
			if item.InspectedAt.IsZero() || item.ValidUntil.IsZero() {
				return "", nil, Unprocessable("inspected_at", "必须提供检查时间和有效期", map[string]any{"check_code": item.CheckCode})
			}
			if item.InspectedAt.After(s.now().UTC()) {
				return "", nil, Unprocessable("inspected_at", "检查时间不能晚于当前时间", map[string]any{"check_code": item.CheckCode})
			}
			if item.ValidUntil.Before(m.WindowEnd) {
				return "", nil, Unprocessable("valid_until", "装备证据有效期必须覆盖任务结束时间", map[string]any{"check_code": item.CheckCode, "required_until": m.WindowEnd})
			}
			if item.ValidUntil.Before(s.now().UTC()) {
				return "", nil, Unprocessable("valid_until", "装备证据已经过期", map[string]any{"check_code": item.CheckCode})
			}
			digest := strings.ToLower(item.EvidenceDigest)
			if code, used := digests[digest]; used {
				return "", nil, Unprocessable("evidence_digest", "证据摘要已绑定到其他检查项", map[string]any{"check_code": item.CheckCode, "bound_check_code": code})
			}
			calculatedOutcome := item.Outcome
			var measurementResults []MeasurementResult
			if len(item.Measurements) > 0 {
				var measurementErr error
				measurementResults, calculatedOutcome, measurementErr = evaluateMeasurements(m, item.CheckCode, item.Measurements)
				if measurementErr != nil {
					return "", nil, measurementErr
				}
			}
			if calculatedOutcome == "" {
				return "", nil, Unprocessable("measurements", "必须提交量化读数", map[string]any{"check_code": item.CheckCode})
			}
			if calculatedOutcome == "fail" && len(item.Measurements) == 0 && (strings.TrimSpace(item.ReviewMarker) == "" || strings.TrimSpace(item.FailureReason) == "") {
				return "", nil, Unprocessable("failure_reason", "失败项必须提供失败原因和可追踪复核标记", map[string]any{"check_code": item.CheckCode})
			}
			replacementFor := strings.ToLower(strings.TrimSpace(item.ReplacementForAssetID))
			if replacementFor != "" {
				failed, ok := failedAssets[replacementFor]
				if !ok || failed.CheckCode != item.CheckCode || calculatedOutcome != "pass" {
					return "", nil, Unprocessable("replacement_for_asset_id", "替换必须引用同一检查项的失败资产", map[string]any{"check_code": item.CheckCode, "replacement_for_asset_id": replacementFor})
				}
				if replacementFor == assetID {
					return "", nil, Unprocessable("asset_id", "替换资产不得与失败资产相同", map[string]any{"check_code": item.CheckCode, "asset_id": assetID})
				}
				if strings.TrimSpace(item.ReplacementReason) == "" {
					return "", nil, Unprocessable("replacement_reason", "替换通过记录必须填写替换理由", map[string]any{"check_code": item.CheckCode})
				}
			} else if current, ok := latestRecords(m.Verifications, "equipment")[item.CheckCode]; ok && current.Outcome == "fail" && calculatedOutcome == "pass" {
				return "", nil, Unprocessable("replacement_for_asset_id", "失败后的通过记录必须引用被替换资产", map[string]any{"check_code": item.CheckCode, "failed_asset_id": current.AssetID})
			}
			digests[digest] = item.CheckCode
			assets[assetID] = item.CheckCode
			records = append(records, VerificationRecord{ID: newID("verification"), MissionID: m.ID, RecordType: "equipment", CheckCode: item.CheckCode, Outcome: calculatedOutcome, EvidenceDigest: digest, ReviewMarker: strings.TrimSpace(item.ReviewMarker), FailureReason: strings.TrimSpace(item.FailureReason), ReplacementForAssetID: replacementFor, ReplacementReason: strings.TrimSpace(item.ReplacementReason), VerifiedBy: in.Meta.ActorID, RecordedAt: s.now().UTC(), AssetID: assetID, InspectedAt: item.InspectedAt.UTC(), ValidUntil: item.ValidUntil.UTC(), Measurements: item.Measurements, MeasurementResults: measurementResults, RuleVersion: equipmentRuleVersion})
		}
		replaced := []VerificationRecord{}
		for _, record := range records {
			if old, ok := latestRecords(m.Verifications, "equipment")[record.CheckCode]; ok {
				m.VerificationHistory = append(m.VerificationHistory, old)
				replaced = append(replaced, old)
			}
			m.Verifications = replaceRecord(m.Verifications, record)
		}
		if EquipmentCompleteAt(m, s.now().UTC()) {
			if DrillsComplete(m) && len(unresolvedDeviations(m)) == 0 {
				m.Status = StatusReadyForRelease
			} else {
				m.Status = StatusDrillPending
			}
		}
		eventType := "equipment_verified"
		replacementChain := []VerificationRecord{}
		for _, record := range records {
			if record.ReplacementForAssetID != "" {
				replacementChain = append(replacementChain, record)
			}
		}
		if len(replacementChain) > 0 {
			eventType = "equipment_replaced"
		} else if refreshMode {
			eventType = "equipment_evidence_refreshed"
		}
		return eventType, struct {
			Records            []VerificationRecord           `json:"records"`
			Verified           []string                       `json:"verified"`
			Passed             []string                       `json:"passed"`
			Missing            []string                       `json:"missing"`
			Expired            []string                       `json:"expired"`
			Replaced           []VerificationRecord           `json:"replaced"`
			NewStatus          Status                         `json:"new_status"`
			ReplacementChain   []VerificationRecord           `json:"replacement_chain"`
			MeasurementResults map[string][]MeasurementResult `json:"measurement_results"`
			FailedMeasurements map[string][]MeasurementResult `json:"failed_measurements"`
		}{records, equipmentCodes(m, true), equipmentCodes(m, true), equipmentCodes(m, false), expiredEquipmentCodesAt(m, s.now().UTC()), replaced, m.Status, replacementChain, measurementResultsByCode(records), failedMeasurementResultsByCode(records)}, nil
	})
}

func expiredEquipmentCodes(m *DiveMission) []string {
	return expiredEquipmentCodesAt(m, time.Now().UTC())
}
func expiredEquipmentCodesAt(m *DiveMission, now time.Time) []string {
	var out []string
	for code, r := range latestRecords(m.Verifications, "equipment") {
		if r.ValidUntil.Before(m.WindowEnd) || r.ValidUntil.Before(now) {
			out = append(out, code)
		}
	}
	sort.Strings(out)
	return out
}

func equipmentCodes(m *DiveMission, passed bool) []string {
	records := latestRecords(m.Verifications, "equipment")
	var result []string
	for code := range equipmentLabels {
		if (records[code].Outcome == "pass") == passed {
			result = append(result, code)
		}
	}
	sort.Strings(result)
	return result
}

func (s *Service) RecordDrill(ctx context.Context, id string, in DrillInput) (StoredResult, bool, error) {
	return s.RecordDrillBatch(ctx, id, DrillBatchInput{Meta: in.Meta, Items: []DrillInput{in}})
}

func (s *Service) RecordDrillBatch(ctx context.Context, id string, in DrillBatchInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "record_drill_batch", in.Meta, in, func(_ context.Context, _ Tx, m *DiveMission) (string, any, error) {
		if m.Status != StatusDrillPending {
			return "", nil, InvalidState(m.Status, "记录演练")
		}
		if len(in.Items) == 0 {
			return "", nil, Invalid("items", "至少包含一个演练项")
		}
		if !EquipmentCompleteAt(m, s.now().UTC()) {
			return "", nil, Unprocessable("equipment", "装备证据已过期或不完整", map[string]any{"expired": expiredEquipmentCodes(m)})
		}
		var equipmentCompleted time.Time
		for _, r := range latestRecords(m.Verifications, "equipment") {
			if r.RecordedAt.After(equipmentCompleted) {
				equipmentCompleted = r.RecordedAt
			}
		}
		seen := map[string]bool{}
		evidenceOwners := map[string]string{}
		for _, record := range append(append([]VerificationRecord(nil), m.Verifications...), m.VerificationHistory...) {
			if record.RecordType == "drill" || record.RecordType == "remediation" || record.RecordType == "retest" {
				evidenceOwners[strings.ToLower(record.EvidenceDigest)] = record.CheckCode
			}
		}
		records := make([]VerificationRecord, 0, len(in.Items))
		for _, item := range in.Items {
			if _, ok := drillLabels[item.CheckCode]; !ok {
				return "", nil, Invalid("check_code", "不是规定演练代码")
			}
			if seen[item.CheckCode] {
				return "", nil, Unprocessable("items", "批次内 check_code 必须唯一", map[string]any{"check_code": item.CheckCode})
			}
			seen[item.CheckCode] = true
			if item.Outcome != "" && item.Outcome != "pass" && item.Outcome != "deviation" {
				return "", nil, Invalid("outcome", "如提供必须为 pass 或 deviation")
			}
			if !validEvidence(item.EvidenceDigest) {
				return "", nil, Invalid("evidence_digest", "必须为十六进制摘要")
			}
			digest := strings.ToLower(item.EvidenceDigest)
			if code, used := evidenceOwners[digest]; used {
				return "", nil, Unprocessable("evidence_digest", "证据摘要在本任务演练、整改和复验记录中必须唯一", map[string]any{"check_code": item.CheckCode, "conflict_check_code": code})
			}
			evidenceOwners[digest] = item.CheckCode
			if item.ConductedAt.IsZero() || item.ConductedAt.After(s.now().UTC()) {
				return "", nil, Unprocessable("conducted_at", "演练时间必须提供且不能晚于当前时间", map[string]any{"check_code": item.CheckCode})
			}
			if item.ConductedAt.Before(equipmentCompleted) {
				return "", nil, Unprocessable("conducted_at", "演练不能早于本轮装备全部核验完成时间", map[string]any{"check_code": item.CheckCode, "equipment_completed_at": equipmentCompleted})
			}
			if item.ConductedAt.Before(m.WindowStart) || item.ConductedAt.After(m.WindowEnd) {
				return "", nil, Unprocessable("conducted_at", "演练时间必须位于任务时间窗内", map[string]any{"check_code": item.CheckCode, "window_start": m.WindowStart, "window_end": m.WindowEnd})
			}
			duration := item.ObservedDurationSeconds
			if duration == 0 {
				duration = item.DurationSeconds
			}
			if duration <= 0 {
				return "", nil, Unprocessable("duration_seconds", "演练持续时长必须为正数", map[string]any{"check_code": item.CheckCode})
			}
			allowedSteps, completed := map[string]bool{}, map[string]bool{}
			for _, step := range drillRequiredSteps[item.CheckCode] {
				allowedSteps[step] = true
			}
			for _, step := range item.CompletedSteps {
				if !allowedSteps[step] {
					return "", nil, Unprocessable("completed_steps", "包含未知演练步骤", map[string]any{"check_code": item.CheckCode, "step": step, "allowed": drillRequiredSteps[item.CheckCode]})
				}
				if completed[step] {
					return "", nil, Unprocessable("completed_steps", "演练步骤不能重复", map[string]any{"check_code": item.CheckCode, "step": step})
				}
				completed[step] = true
			}
			if in.Meta.ActorID == m.LeaderID || (m.LifeSupportPlan != nil && in.Meta.ActorID == m.LifeSupportPlan.ReviewedBy) {
				return "", nil, Unprocessable("actor_id", "演练见证人必须独立于领队和方案审核员", map[string]any{"actor_id": in.Meta.ActorID})
			}
			for _, record := range m.Verifications {
				if record.RecordType == "equipment" && record.VerifiedBy == in.Meta.ActorID {
					return "", nil, Unprocessable("actor_id", "演练见证人不能是装备核验员", map[string]any{"actor_id": in.Meta.ActorID})
				}
			}
			limit := drillDurationLimit(item.CheckCode, m.RiskSummary.HighestLevel)
			deviationCodes := []string{}
			deviationParts := []string{}
			if duration > limit {
				deviationCodes = append(deviationCodes, "duration_exceeded")
				deviationParts = append(deviationParts, "超时 "+strconv.Itoa(duration-limit)+" 秒")
			}
			missing := []string{}
			for _, step := range drillRequiredSteps[item.CheckCode] {
				if !completed[step] {
					missing = append(missing, step)
				}
			}
			if len(missing) > 0 {
				deviationCodes = append(deviationCodes, "required_steps_missing")
				deviationParts = append(deviationParts, "缺少步骤: "+strings.Join(missing, ","))
			}
			outcome := "pass"
			if len(deviationCodes) > 0 {
				outcome = "deviation"
			}
			if note := strings.TrimSpace(item.Deviation); note != "" {
				deviationParts = append(deviationParts, note)
			}
			steps := append([]string(nil), item.CompletedSteps...)
			sort.Strings(steps)
			record := VerificationRecord{ID: newID("verification"), MissionID: m.ID, RecordType: "drill", CheckCode: item.CheckCode,
				Outcome: outcome, EvidenceDigest: digest, Deviation: strings.Join(deviationParts, "; "), DeviationCodes: deviationCodes, VerifiedBy: in.Meta.ActorID, RecordedAt: s.now().UTC(), ConductedAt: item.ConductedAt.UTC(), DurationSeconds: duration, CompletedSteps: steps, RuleVersion: drillRuleVersion, RequiredMaxDurationSeconds: limit}
			if outcome == "deviation" {
				record.RemediationDueAt = remediationDueAt(m, record)
			}
			records = append(records, record)
		}
		for _, record := range records {
			if old, ok := latestRecords(m.Verifications, "drill")[record.CheckCode]; ok {
				m.VerificationHistory = append(m.VerificationHistory, old)
			}
			m.Verifications = replaceRecord(m.Verifications, record)
		}
		drills := latestRecords(m.Verifications, "drill")
		if len(drills) == len(drillLabels) {
			if len(unresolvedDeviations(m)) > 0 {
				m.Status = StatusRemediation
			} else {
				m.Status = StatusReadyForRelease
			}
		}
		completed := make([]string, 0, len(drills))
		for code := range drills {
			completed = append(completed, code)
		}
		sort.Strings(completed)
		pending := []string{}
		for code := range drillLabels {
			if _, ok := drills[code]; !ok {
				pending = append(pending, code)
			}
		}
		sort.Strings(pending)
		checks := make([]map[string]any, 0, len(records))
		for _, record := range records {
			checks = append(checks, map[string]any{"check_code": record.CheckCode, "calculated_outcome": record.Outcome, "rule_version": record.RuleVersion, "required_max_duration_seconds": record.RequiredMaxDurationSeconds, "observed_duration_seconds": record.DurationSeconds, "threshold_result": map[string]any{"passed": record.DurationSeconds <= record.RequiredMaxDurationSeconds, "exceeded_seconds": max(0, record.DurationSeconds-record.RequiredMaxDurationSeconds)}, "completed_steps": record.CompletedSteps, "deviations": record.DeviationCodes})
		}
		return "drill_batch_recorded", map[string]any{"records": records, "completed": completed, "pending": pending, "deviations": unresolvedDeviations(m), "calculations": checks, "rule_version": drillRuleVersion}, nil
	})
}

func (s *Service) RecordRemediation(ctx context.Context, id string, in RemediationInput) (StoredResult, bool, error) {
	return s.RecordRemediationBatch(ctx, id, RemediationBatchInput{Meta: in.Meta, Items: []RemediationInput{in}})
}

func usedVerificationDigests(m *DiveMission) map[string]string {
	out := map[string]string{}
	for _, r := range append(append([]VerificationRecord(nil), m.Verifications...), m.VerificationHistory...) {
		if (r.RecordType == "drill" || r.RecordType == "remediation" || r.RecordType == "retest") && r.EvidenceDigest != "" {
			out[strings.ToLower(r.EvidenceDigest)] = r.CheckCode
		}
	}
	return out
}

func verificationRecordsFor(m *DiveMission, recordType, checkCode string) []VerificationRecord {
	all := append(append([]VerificationRecord(nil), m.VerificationHistory...), m.Verifications...)
	out := []VerificationRecord{}
	for _, record := range all {
		if record.RecordType == recordType && record.CheckCode == checkCode {
			out = append(out, record)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RemediationCycle == out[j].RemediationCycle {
			return out[i].RecordedAt.Before(out[j].RecordedAt)
		}
		return out[i].RemediationCycle < out[j].RemediationCycle
	})
	return out
}

func latestCycleRecord(m *DiveMission, recordType, checkCode string) (VerificationRecord, bool) {
	records := verificationRecordsFor(m, recordType, checkCode)
	if len(records) == 0 {
		return VerificationRecord{}, false
	}
	return records[len(records)-1], true
}

func cycleStatuses(m *DiveMission) []map[string]any {
	result := []map[string]any{}
	for code := range drillLabels {
		for _, remediation := range verificationRecordsFor(m, "remediation", code) {
			status := "awaiting_retest"
			failureReason := ""
			for _, retest := range verificationRecordsFor(m, "retest", code) {
				if retest.RemediationCycle == remediation.RemediationCycle {
					if retest.Outcome == "pass" {
						status = "passed"
					} else {
						status, failureReason = "failed", retest.Deviation
					}
					break
				}
			}
			result = append(result, map[string]any{"check_code": code, "cycle": remediation.RemediationCycle, "status": status, "remediation_record_id": remediation.ID, "failure_reason": failureReason})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		ci, cj := result[i]["check_code"].(string), result[j]["check_code"].(string)
		if ci == cj {
			return result[i]["cycle"].(int) < result[j]["cycle"].(int)
		}
		return ci < cj
	})
	return result
}

func (s *Service) RecordRemediationBatch(ctx context.Context, id string, in RemediationBatchInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "record_remediation_batch", in.Meta, in, func(_ context.Context, _ Tx, m *DiveMission) (string, any, error) {
		if m.Status != StatusRemediation {
			return "", nil, InvalidState(m.Status, "记录整改")
		}
		drills := latestRecords(m.Verifications, "drill")
		if len(in.Items) == 0 {
			return "", nil, Invalid("items", "至少包含一个整改项")
		}
		seen := map[string]bool{}
		digests := usedVerificationDigests(m)
		records := make([]VerificationRecord, 0, len(in.Items))
		for _, item := range in.Items {
			if seen[item.CheckCode] {
				return "", nil, Unprocessable("items", "批次内 check_code 必须唯一", map[string]any{"check_code": item.CheckCode})
			}
			seen[item.CheckCode] = true
			if drills[item.CheckCode].Outcome != "deviation" {
				return "", nil, Unprocessable("check_code", "该演练不存在待整改偏差", map[string]any{"check_code": item.CheckCode})
			}
			latestRemediation, hasRemediation := latestCycleRecord(m, "remediation", item.CheckCode)
			latestRetest, hasRetest := latestCycleRecord(m, "retest", item.CheckCode)
			cycle := 1
			referenceID := drills[item.CheckCode].ID
			if hasRemediation {
				if !hasRetest || latestRetest.RemediationCycle != latestRemediation.RemediationCycle {
					return "", nil, Unprocessable("remediation_cycle", "最新整改轮次尚未复验", map[string]any{"check_code": item.CheckCode, "cycle": latestRemediation.RemediationCycle})
				}
				if latestRetest.Outcome != "fail" {
					return "", nil, Unprocessable("check_code", "该偏差已经闭环", map[string]any{"check_code": item.CheckCode})
				}
				cycle, referenceID = latestRemediation.RemediationCycle+1, latestRetest.ID
				if strings.TrimSpace(item.CorrectiveAction) == latestRemediation.CorrectiveAction {
					return "", nil, &Error{Code: "remediation_cycle_required", Message: "复验失败后必须提交内容变化的新纠正措施", Status: 422, Details: map[string]any{"check_code": item.CheckCode, "next_cycle": cycle}}
				}
			}
			if item.Cycle > 0 && item.Cycle != cycle {
				return "", nil, Unprocessable("cycle", "整改轮次必须连续递增", map[string]any{"check_code": item.CheckCode, "expected_cycle": cycle})
			}
			if item.ReferencedRecordID != "" && item.ReferencedRecordID != referenceID {
				return "", nil, Unprocessable("referenced_record_id", "整改引用必须指向同代码原偏差或最近失败复验", map[string]any{"check_code": item.CheckCode, "expected_record_id": referenceID})
			}
			if strings.TrimSpace(item.CorrectiveAction) == "" {
				return "", nil, Unprocessable("corrective_action", "必须填写整改说明", map[string]any{"check_code": item.CheckCode})
			}
			if !validEvidence(item.EvidenceDigest) {
				return "", nil, Unprocessable("evidence_digest", "必须为十六进制摘要", map[string]any{"check_code": item.CheckCode})
			}
			digest := strings.ToLower(item.EvidenceDigest)
			if code, ok := digests[digest]; ok {
				return "", nil, Unprocessable("evidence_digest", "证据摘要在本任务核验记录中必须唯一", map[string]any{"check_code": item.CheckCode, "conflict_check_code": code})
			}
			digests[digest] = item.CheckCode
			if drills[item.CheckCode].VerifiedBy == in.Meta.ActorID {
				return "", nil, Unprocessable("actor_id", "整改操作者不能是原演练者", map[string]any{"check_code": item.CheckCode})
			}
			if m.LifeSupportPlan != nil && m.LifeSupportPlan.ReviewedBy == in.Meta.ActorID {
				return "", nil, Unprocessable("actor_id", "整改操作者不能是方案审核员", map[string]any{"check_code": item.CheckCode})
			}
			if item.CompletedAt.IsZero() {
				return "", nil, Unprocessable("completed_at", "必须提供整改完成时间", map[string]any{"check_code": item.CheckCode})
			}
			completedAt := item.CompletedAt.UTC()
			if completedAt.After(s.now().UTC()) {
				return "", nil, Unprocessable("completed_at", "整改完成时间不能晚于当前时间", map[string]any{"check_code": item.CheckCode})
			}
			if completedAt.Before(drills[item.CheckCode].RecordedAt) {
				return "", nil, Unprocessable("completed_at", "整改完成时间不能早于偏差记录时间", map[string]any{"check_code": item.CheckCode, "deviation_recorded_at": drills[item.CheckCode].RecordedAt})
			}
			dueAt := drills[item.CheckCode].RemediationDueAt
			overdue := !dueAt.IsZero() && completedAt.After(dueAt)
			delayReason, delayReviewer := strings.TrimSpace(item.DelayReason), strings.TrimSpace(item.DelayReviewedBy)
			if overdue {
				if delayReason == "" || delayReviewer == "" {
					return "", nil, Unprocessable("delay_reason", "逾期整改必须提供迟延原因和独立安全复核人", map[string]any{"check_code": item.CheckCode, "due_at": dueAt})
				}
				conflict := memberIDs(m)[delayReviewer] || delayReviewer == drills[item.CheckCode].VerifiedBy || delayReviewer == in.Meta.ActorID || m.LifeSupportPlan != nil && delayReviewer == m.LifeSupportPlan.ReviewedBy
				if conflict {
					return "", nil, Unprocessable("delay_reviewed_by", "迟延安全复核人必须与任务、演练、整改和方案审核角色独立", map[string]any{"check_code": item.CheckCode, "delay_reviewed_by": delayReviewer})
				}
			}
			delaySeconds := int64(0)
			if overdue {
				delaySeconds = int64(completedAt.Sub(dueAt).Seconds())
			}
			records = append(records, VerificationRecord{ID: newID("verification"), MissionID: m.ID, RecordType: "remediation", CheckCode: item.CheckCode, Outcome: "completed", EvidenceDigest: digest, CorrectiveAction: strings.TrimSpace(item.CorrectiveAction), VerifiedBy: in.Meta.ActorID, RecordedAt: completedAt, RemediationCycle: cycle, ReferencedRecordID: referenceID, RemediationDueAt: dueAt, WasOverdue: overdue, DelayReason: delayReason, DelayReviewedBy: delayReviewer, DelaySeconds: delaySeconds, RuleVersion: remediationRuleVersion})
		}
		for _, record := range records {
			if old, ok := latestRecords(m.Verifications, "remediation")[record.CheckCode]; ok {
				m.VerificationHistory = append(m.VerificationHistory, old)
			}
			m.Verifications = replaceRecord(m.Verifications, record)
		}
		return "remediation_batch_recorded", map[string]any{"records": records, "unresolved": unresolvedDeviations(m), "cycle_statuses": cycleStatuses(m), "next_allowed_actions": []string{"record_retest"}}, nil
	})
}

func (s *Service) RecordRetest(ctx context.Context, id string, in RetestInput) (StoredResult, bool, error) {
	return s.RecordRetestBatch(ctx, id, RetestBatchInput{Meta: in.Meta, Items: []RetestInput{in}})
}

func (s *Service) RecordRetestBatch(ctx context.Context, id string, in RetestBatchInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "record_retest_batch", in.Meta, in, func(_ context.Context, _ Tx, m *DiveMission) (string, any, error) {
		if m.Status != StatusRemediation {
			return "", nil, InvalidState(m.Status, "定向复验")
		}
		remediations := latestRecords(m.Verifications, "remediation")
		drills := latestRecords(m.Verifications, "drill")
		if len(in.Items) == 0 {
			return "", nil, Invalid("items", "至少包含一个复验项")
		}
		seen := map[string]bool{}
		digests := usedVerificationDigests(m)
		records := make([]VerificationRecord, 0, len(in.Items))
		for _, item := range in.Items {
			if seen[item.CheckCode] {
				return "", nil, Unprocessable("items", "批次内 check_code 必须唯一", map[string]any{"check_code": item.CheckCode})
			}
			seen[item.CheckCode] = true
			if remediations[item.CheckCode].CorrectiveAction == "" {
				return "", nil, Unprocessable("check_code", "必须先完成对应整改", map[string]any{"check_code": item.CheckCode})
			}
			remediation := remediations[item.CheckCode]
			if latestRetest, ok := latestCycleRecord(m, "retest", item.CheckCode); ok && latestRetest.RemediationCycle == remediation.RemediationCycle {
				return "", nil, &Error{Code: "remediation_cycle_required", Message: "该整改轮次已复验，失败后必须先提交新整改", Status: 422, Details: map[string]any{"check_code": item.CheckCode, "consumed_cycle": remediation.RemediationCycle}}
			}
			if item.Cycle > 0 && item.Cycle != remediation.RemediationCycle {
				return "", nil, Unprocessable("cycle", "复验必须引用最新未消费整改轮次", map[string]any{"check_code": item.CheckCode, "expected_cycle": remediation.RemediationCycle})
			}
			if item.ReferencedRecordID != "" && item.ReferencedRecordID != remediation.ID {
				return "", nil, Unprocessable("referenced_record_id", "复验必须引用同代码最新整改记录", map[string]any{"check_code": item.CheckCode, "expected_record_id": remediation.ID})
			}
			if item.Outcome != "pass" && item.Outcome != "fail" {
				return "", nil, Unprocessable("outcome", "必须为 pass 或 fail", map[string]any{"check_code": item.CheckCode})
			}
			if !validEvidence(item.EvidenceDigest) {
				return "", nil, Unprocessable("evidence_digest", "必须为十六进制摘要", map[string]any{"check_code": item.CheckCode})
			}
			digest := strings.ToLower(item.EvidenceDigest)
			if code, ok := digests[digest]; ok {
				return "", nil, Unprocessable("evidence_digest", "证据摘要在本任务核验记录中必须唯一", map[string]any{"check_code": item.CheckCode, "conflict_check_code": code})
			}
			digests[digest] = item.CheckCode
			if remediations[item.CheckCode].VerifiedBy == in.Meta.ActorID {
				return "", nil, Unprocessable("actor_id", "复验操作者不能是整改者", map[string]any{"check_code": item.CheckCode})
			}
			if drills[item.CheckCode].VerifiedBy == in.Meta.ActorID {
				return "", nil, Unprocessable("actor_id", "复验操作者不能是原演练者", map[string]any{"check_code": item.CheckCode})
			}
			if m.LifeSupportPlan != nil && m.LifeSupportPlan.ReviewedBy == in.Meta.ActorID {
				return "", nil, Unprocessable("actor_id", "复验操作者不能是方案审核员", map[string]any{"check_code": item.CheckCode})
			}
			deviation := ""
			if item.Outcome == "fail" {
				deviation = "定向复验未通过"
			}
			records = append(records, VerificationRecord{ID: newID("verification"), MissionID: m.ID, RecordType: "retest", CheckCode: item.CheckCode, Outcome: item.Outcome, EvidenceDigest: digest, Deviation: deviation, VerifiedBy: in.Meta.ActorID, RecordedAt: s.now().UTC(), RemediationCycle: remediation.RemediationCycle, ReferencedRecordID: remediation.ID})
		}
		for _, record := range records {
			if old, ok := latestRecords(m.Verifications, "retest")[record.CheckCode]; ok {
				m.VerificationHistory = append(m.VerificationHistory, old)
			}
			m.Verifications = replaceRecord(m.Verifications, record)
		}
		if len(unresolvedDeviations(m)) == 0 {
			m.Status = StatusReadyForRelease
		}
		return "retest_batch_recorded", map[string]any{"records": records, "unresolved": unresolvedDeviations(m), "new_status": m.Status, "cycle_statuses": cycleStatuses(m), "next_allowed_actions": AllowedActions(m)}, nil
	})
}
