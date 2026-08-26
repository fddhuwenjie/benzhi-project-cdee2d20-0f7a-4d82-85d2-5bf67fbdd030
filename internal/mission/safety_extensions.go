package mission

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	qualificationRuleVersion = "member-qualification-v1"
	gasBudgetRuleVersion     = "segment-gas-budget-v1"
	equipmentRuleVersion     = "equipment-measurement-v1"
	remediationRuleVersion   = "drill-remediation-deadline-v1"
)

var roleQualificationCode = map[string]string{
	"leader": "cave_leader", "support": "cave_support", "standby": "standby_rescue",
}

func requiredQualificationLevel(depth float64) int {
	switch {
	case depth <= 30:
		return 1
	case depth <= 60:
		return 2
	case depth <= 100:
		return 3
	default:
		return 4
	}
}

func ValidateQualifications(members []Member, qualifications []MemberQualification, depth float64, windowEnd time.Time) ([]MemberQualification, []QualificationStatus, error) {
	team := make(map[string]Member, len(members))
	for _, member := range members {
		team[member.PersonID] = member
	}
	if len(qualifications) != len(members) {
		return nil, nil, Unprocessable("member_qualifications", "资格人员必须与 team_members 一一对应", map[string]any{"required_count": len(members), "actual_count": len(qualifications)})
	}
	seenPeople, seenEvidence := map[string]bool{}, map[string]bool{}
	canonical := append([]MemberQualification(nil), qualifications...)
	statuses := make([]QualificationStatus, 0, len(members))
	byPerson := map[string]MemberQualification{}
	for i := range canonical {
		q := &canonical[i]
		if q.Level == 0 {
			q.Level = q.LegacyLevel
		}
		q.LegacyLevel = 0
		q.PersonID = strings.TrimSpace(q.PersonID)
		q.QualificationCode = strings.ToLower(strings.TrimSpace(q.QualificationCode))
		q.EvidenceDigest = strings.ToLower(strings.TrimSpace(q.EvidenceDigest))
		member, ok := team[q.PersonID]
		if !ok || seenPeople[q.PersonID] {
			return nil, nil, Unprocessable("member_qualifications", "资格人员必须与任务成员唯一对应", map[string]any{"person_id": q.PersonID})
		}
		seenPeople[q.PersonID] = true
		requiredCode, requiredLevel := roleQualificationCode[member.Role], requiredQualificationLevel(depth)
		codeMatches := q.QualificationCode == requiredCode || q.QualificationCode == member.Role || q.QualificationCode == member.Role+"_qualification"
		if !codeMatches || q.Level < requiredLevel || q.Level > 4 || q.ValidUntil.Before(windowEnd) {
			missing := []string{}
			if !codeMatches || q.Level < requiredLevel {
				missing = append(missing, requiredCode)
			}
			return nil, nil, Unprocessable("member_qualifications", "成员岗位资格不能覆盖任务要求", map[string]any{"person_id": q.PersonID, "missing_qualifications": missing, "required_code": requiredCode, "required_level": requiredLevel, "required_valid_until": windowEnd})
		}
		if !validEvidence(q.EvidenceDigest) || seenEvidence[q.EvidenceDigest] {
			return nil, nil, Unprocessable("evidence_digest", "资格证据摘要格式无效或在任务内重复", map[string]any{"person_id": q.PersonID})
		}
		seenEvidence[q.EvidenceDigest] = true
		byPerson[q.PersonID] = *q
	}
	for _, member := range members {
		q, ok := byPerson[member.PersonID]
		if !ok {
			return nil, nil, Unprocessable("member_qualifications", "缺少任务成员资格", map[string]any{"person_id": member.PersonID, "missing_qualifications": []string{roleQualificationCode[member.Role]}})
		}
		statuses = append(statuses, QualificationStatus{PersonID: member.PersonID, Role: member.Role, Passed: true, RequiredCode: roleQualificationCode[member.Role], RequiredLevel: requiredQualificationLevel(depth), EvidenceDigest: q.EvidenceDigest, RemainingValidDays: int64(q.ValidUntil.Sub(windowEnd).Hours() / 24)})
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].PersonID < canonical[j].PersonID })
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].PersonID < statuses[j].PersonID })
	return canonical, statuses, nil
}

func validateMitigationActions(m *DiveMission, risks []SegmentRisk) ([]SegmentRisk, error) {
	team := memberIDs(m)
	seenCodes := map[string]bool{}
	seenEvidence := map[string]bool{}
	for _, previous := range append(append([]SegmentRisk(nil), m.RiskHistory...), m.Risks...) {
		for _, action := range previous.MitigationActions {
			seenCodes[action.Code] = true
			if action.EvidenceDigest != "" {
				seenEvidence[action.EvidenceDigest] = true
			}
		}
	}
	result := append([]SegmentRisk(nil), risks...)
	for i := range result {
		risk := &result[i]
		if risk.RiskLevel != "high" && risk.RiskLevel != "critical" {
			continue
		}
		covered := map[string]bool{}
		for j := range risk.MitigationActions {
			a := &risk.MitigationActions[j]
			if a.Code == "" {
				a.Code = a.LegacyCode
			}
			a.LegacyCode = ""
			a.Code = strings.ToLower(strings.TrimSpace(a.Code))
			a.Hazard = strings.TrimSpace(a.Hazard)
			a.OwnerPersonID = strings.TrimSpace(a.OwnerPersonID)
			a.CompletionCriteria = strings.TrimSpace(a.CompletionCriteria)
			if a.Code == "" || seenCodes[a.Code] {
				return nil, Unprocessable("mitigation_actions", "缓解行动代码必须在任务内唯一", map[string]any{"segment_name": risk.SegmentName, "action_code": a.Code})
			}
			seenCodes[a.Code] = true
			if !team[a.OwnerPersonID] {
				return nil, Unprocessable("owner_person_id", "缓解负责人必须是当前任务成员", map[string]any{"action_code": a.Code, "person_id": a.OwnerPersonID})
			}
			if a.DueAt.IsZero() || !a.DueAt.Before(m.WindowStart) {
				return nil, Unprocessable("due_at", "缓解期限必须早于任务开始", map[string]any{"action_code": a.Code, "required_before": m.WindowStart})
			}
			hazardKnown := false
			for _, hazard := range risk.Hazards {
				hazardKnown = hazardKnown || strings.TrimSpace(hazard) == a.Hazard
			}
			if !hazardKnown || len([]rune(a.CompletionCriteria)) < 8 {
				return nil, Unprocessable("mitigation_actions", "行动必须对应危险项并提供具体完成标准", map[string]any{"action_code": a.Code, "hazard": a.Hazard})
			}
			if a.Status == "" {
				a.Status = "open"
			}
			if a.Status != "open" || a.Result != "" || a.EvidenceDigest != "" {
				return nil, Unprocessable("mitigation_actions", "初始缓解行动状态必须为 open", map[string]any{"action_code": a.Code})
			}
			a.Version = 1
			covered[a.Hazard] = true
		}
		for _, hazard := range risk.Hazards {
			if !covered[strings.TrimSpace(hazard)] {
				return nil, Unprocessable("mitigation_actions", "高风险洞段每个危险项都必须有缓解行动", map[string]any{"segment_name": risk.SegmentName, "hazard": hazard})
			}
		}
		sort.Slice(risk.MitigationActions, func(i, j int) bool { return risk.MitigationActions[i].Code < risk.MitigationActions[j].Code })
		for _, a := range risk.MitigationActions {
			if a.EvidenceDigest != "" {
				if seenEvidence[a.EvidenceDigest] {
					return nil, Unprocessable("evidence_digest", "缓解证据摘要必须全任务唯一", nil)
				}
				seenEvidence[a.EvidenceDigest] = true
			}
		}
	}
	return result, nil
}

func hasStructuredMitigations(risks []SegmentRisk) bool {
	for _, risk := range risks {
		if risk.MitigationActions != nil {
			return true
		}
	}
	return false
}

func RequiresMitigationActions(risk SegmentRisk) bool {
	scored := scoreRisk(risk)
	return scored.RiskLevel == "high" || scored.RiskLevel == "critical"
}

func linkMitigationVersions(previous, next []SegmentRisk) {
	oldByHazard := map[string]MitigationAction{}
	for _, risk := range previous {
		for _, action := range risk.MitigationActions {
			oldByHazard[risk.SegmentName+"\x00"+action.Hazard] = action
		}
	}
	for i := range next {
		for j := range next[i].MitigationActions {
			if old, ok := oldByHazard[next[i].SegmentName+"\x00"+next[i].MitigationActions[j].Hazard]; ok {
				next[i].MitigationActions[j].PreviousActionCode = old.Code
				next[i].MitigationActions[j].Version = old.Version + 1
			}
		}
	}
}

func (s *Service) CompleteMitigation(ctx context.Context, id string, in MitigationCompletionInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "complete_risk_mitigation", in.Meta, in, func(_ context.Context, _ Tx, m *DiveMission) (string, any, error) {
		if m.Status != StatusRiskAssessed {
			return "", nil, InvalidState(m.Status, "完成风险缓解行动")
		}
		code := strings.ToLower(strings.TrimSpace(in.ActionCode))
		if code == "" {
			return "", nil, Invalid("action_code", "不能为空")
		}
		if strings.TrimSpace(in.Result) == "" {
			return "", nil, Invalid("result", "必须提交结果说明")
		}
		digest := strings.ToLower(strings.TrimSpace(in.EvidenceDigest))
		if !validEvidence(digest) {
			return "", nil, Invalid("evidence_digest", "必须为 16 到 128 位十六进制摘要")
		}
		for _, risk := range append(append([]SegmentRisk(nil), m.RiskHistory...), m.Risks...) {
			for _, action := range risk.MitigationActions {
				if action.EvidenceDigest == digest {
					return "", nil, Unprocessable("evidence_digest", "缓解证据摘要必须全任务唯一", map[string]any{"action_code": action.Code})
				}
			}
		}
		completedAt := in.CompletedAt.UTC()
		if in.CompletedAt.IsZero() {
			completedAt = s.now().UTC()
		}
		if completedAt.After(s.now().UTC()) {
			return "", nil, Unprocessable("completed_at", "完成时间不能晚于当前时间", nil)
		}
		var completed *MitigationAction
		segmentName := ""
		for i := range m.Risks {
			for j := range m.Risks[i].MitigationActions {
				action := &m.Risks[i].MitigationActions[j]
				if action.Code != code {
					continue
				}
				if in.Meta.ActorID != action.OwnerPersonID {
					return "", nil, Unprocessable("actor_id", "缓解行动只能由登记负责人完成", map[string]any{"action_code": code, "owner_person_id": action.OwnerPersonID})
				}
				if action.Status == "completed" {
					return "", nil, &Error{Code: "mitigation_already_completed", Message: "已完成行动不可覆盖", Status: 409, Details: map[string]any{"action_code": code}}
				}
				action.Status, action.Result, action.EvidenceDigest, action.CompletedAt = "completed", strings.TrimSpace(in.Result), digest, &completedAt
				completed, segmentName = action, m.Risks[i].SegmentName
			}
		}
		if completed == nil {
			return "", nil, NewError("mitigation_not_found", "缓解行动不存在", 404)
		}
		return "mitigation_completed", map[string]any{"segment_name": segmentName, "action": completed, "remaining_blockers": mitigationBlockers(m, s.now().UTC())}, nil
	})
}

func mitigationBlockers(m *DiveMission, now time.Time) []map[string]any {
	var blockers []map[string]any
	for _, risk := range m.Risks {
		for _, action := range risk.MitigationActions {
			if action.Status != "completed" {
				blockers = append(blockers, map[string]any{"segment_name": risk.SegmentName, "action_code": action.Code, "hazard": action.Hazard, "owner_person_id": action.OwnerPersonID, "due_at": action.DueAt, "status": action.Status, "overdue": now.After(action.DueAt), "action": "complete_risk_mitigation"})
			}
		}
	}
	sort.Slice(blockers, func(i, j int) bool {
		a, b := blockers[i]["segment_name"].(string), blockers[j]["segment_name"].(string)
		if a == b {
			return blockers[i]["action_code"].(string) < blockers[j]["action_code"].(string)
		}
		return a < b
	})
	return blockers
}

func calculateSegmentGasScenarios(m *DiveMission, in PlanInput) ([]GasFailureScenario, error) {
	if len(in.SegmentGasBudgets) == 0 {
		return nil, nil
	}
	if len(in.SegmentGasBudgets) != len(m.Segments) {
		return nil, Unprocessable("segment_gas_budgets", "洞段预算必须完整覆盖任务路线", map[string]any{"required_segments": m.Segments})
	}
	assignments := map[string]MemberGasAssignment{}
	for _, a := range in.MemberGasAssignments {
		assignments[a.PersonID] = a
	}
	mixes := map[string]GasMix{}
	for _, mix := range in.GasMixes {
		mixes[strings.ToLower(mix.Name)] = mix
	}
	riskBySegment := map[string]SegmentRisk{}
	for _, risk := range m.Risks {
		riskBySegment[risk.SegmentName] = risk
	}
	riskMultiplier := map[string]float64{"low": 1, "medium": 1.15, "high": 1.35, "critical": 1.6}[m.RiskSummary.HighestLevel]
	if riskMultiplier == 0 {
		riskMultiplier = 1
	}
	cumulative := map[string]float64{}
	var out []GasFailureScenario
	for i, budget := range in.SegmentGasBudgets {
		if budget.SegmentName != m.Segments[i] || budget.ExpectedDepthM <= 0 || budget.ExpectedDepthM > m.TargetDepthM+0.01 || budget.OneWayMinutes <= 0 || budget.OneWayMinutes > 240 {
			return nil, Unprocessable("segment_gas_budgets", "洞段名称、顺序、深度或单程时间无效", map[string]any{"index": i, "segment_name": budget.SegmentName, "expected_segment": m.Segments[i]})
		}
		for personID, assignment := range assignments {
			multiplier := budget.ConsumptionMultipliers[personID]
			if multiplier < 0.5 || multiplier > 5 {
				return nil, Unprocessable("consumption_multipliers", "成员耗气倍率必须在 0.5 到 5 之间", map[string]any{"person_id": personID, "segment_name": budget.SegmentName})
			}
			if sources := budget.AvailableSources[personID]; len(sources) != 2 || !containsString(sources, "primary") || !containsString(sources, "redundant") {
				return nil, Unprocessable("available_sources", "每名入水成员必须声明主用和冗余气源", map[string]any{"person_id": personID, "segment_name": budget.SegmentName})
			}
			ambient := 1 + budget.ExpectedDepthM/10
			cumulative[personID] += assignment.SurfaceConsumptionLMin * ambient * budget.OneWayMinutes * multiplier
			exitMinutes := float64(riskBySegment[budget.SegmentName].ExitLimitMin)
			required := (cumulative[personID] + assignment.SurfaceConsumptionLMin*ambient*exitMinutes*multiplier) * riskMultiplier
			for _, failed := range []string{"primary", "redundant"} {
				mixName := assignment.RedundantGasMix
				if failed == "redundant" {
					mixName = assignment.PrimaryGasMix
				}
				mix := mixes[strings.ToLower(mixName)]
				available := mix.CylinderLiters * float64(max(0, mix.StartPressureBar-in.TurnPressureBar))
				if in.ReserveRule == "rule_of_thirds" {
					available = math.Min(available, mix.CylinderLiters*float64(mix.StartPressureBar)*2/3)
				}
				margin := round2(available - required)
				scenario := GasFailureScenario{PersonID: personID, SegmentName: budget.SegmentName, FailedSource: failed, RequiredLiters: round2(required), AvailableLiters: round2(available), MarginLiters: margin, Passed: margin >= 0}
				if !scenario.Passed {
					return nil, Unprocessable("segment_gas_budgets", "逐洞段失气撤离预算不足", map[string]any{"person_id": personID, "segment_name": budget.SegmentName, "failed_source": failed, "deficit_liters": round2(-margin), "scenario": scenario})
				}
				out = append(out, scenario)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PersonID != out[j].PersonID {
			return out[i].PersonID < out[j].PersonID
		}
		if out[i].SegmentName != out[j].SegmentName {
			return segmentIndex(m.Segments, out[i].SegmentName) < segmentIndex(m.Segments, out[j].SegmentName)
		}
		return out[i].FailedSource < out[j].FailedSource
	})
	return out, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func segmentIndex(values []string, wanted string) int {
	for i, value := range values {
		if value == wanted {
			return i
		}
	}
	return len(values)
}

func measurementThresholds(m *DiveMission, code string) map[string]Measurement {
	maxExit, routeLength := 0, float64(len(m.Segments))*100
	for _, risk := range m.Risks {
		if risk.ExitLimitMin > maxExit {
			maxExit = risk.ExitLimitMin
		}
	}
	riskExtra := map[string]float64{"low": 0, "medium": 15, "high": 30, "critical": 60}[m.RiskSummary.HighestLevel]
	switch code {
	case "primary_breathing", "backup_breathing":
		return map[string]Measurement{"start_pressure_bar": {Value: math.Max(180, 150+m.TargetDepthM), Unit: "bar"}, "leak_drop_bar": {Value: 5, Unit: "bar"}}
	case "primary_lighting":
		return map[string]Measurement{"runtime_minutes": {Value: float64(maxExit) + riskExtra, Unit: "min"}}
	case "guideline":
		return map[string]Measurement{"usable_length_m": {Value: routeLength, Unit: "m"}, "tensile_strength_n": {Value: 500 + riskExtra*5, Unit: "N"}}
	default:
		return map[string]Measurement{"coverage_distance_m": {Value: routeLength, Unit: "m"}, "battery_percent": {Value: 50 + riskExtra/3, Unit: "%"}}
	}
}

func evaluateMeasurements(m *DiveMission, code string, measurements map[string]Measurement) ([]MeasurementResult, string, error) {
	thresholds := measurementThresholds(m, code)
	if len(measurements) != len(thresholds) {
		return nil, "", Unprocessable("measurements", "缺少必需读数或包含不匹配指标", map[string]any{"check_code": code, "required": thresholds})
	}
	results := make([]MeasurementResult, 0, len(thresholds))
	outcome := "pass"
	for name, threshold := range thresholds {
		reading, ok := measurements[name]
		if !ok || reading.Unit != threshold.Unit || math.IsNaN(reading.Value) || math.IsInf(reading.Value, 0) || reading.Value < 0 || reading.Value > 100000 {
			return nil, "", Unprocessable("measurements", "读数名称、单位或数值无效", map[string]any{"check_code": code, "measurement": name, "required_unit": threshold.Unit})
		}
		operator, margin, passed := ">=", reading.Value-threshold.Value, reading.Value >= threshold.Value
		if name == "leak_drop_bar" {
			operator, margin, passed = "<=", threshold.Value-reading.Value, reading.Value <= threshold.Value
		}
		if !passed {
			outcome = "fail"
		}
		results = append(results, MeasurementResult{Name: name, Value: reading.Value, Unit: reading.Unit, Threshold: threshold.Value, Operator: operator, Margin: round2(margin), Passed: passed})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results, outcome, nil
}

func measurementResultsByCode(records []VerificationRecord) map[string][]MeasurementResult {
	result := map[string][]MeasurementResult{}
	for _, record := range records {
		result[record.CheckCode] = record.MeasurementResults
	}
	return result
}

func failedMeasurementResultsByCode(records []VerificationRecord) map[string][]MeasurementResult {
	result := map[string][]MeasurementResult{}
	for _, record := range records {
		for _, measurement := range record.MeasurementResults {
			if !measurement.Passed {
				result[record.CheckCode] = append(result[record.CheckCode], measurement)
			}
		}
	}
	return result
}

func remediationDueAt(m *DiveMission, record VerificationRecord) time.Time {
	hours := 24
	if m.RiskSummary.HighestLevel == "high" {
		hours = 12
	}
	if m.RiskSummary.HighestLevel == "critical" {
		hours = 6
	}
	if containsString(record.DeviationCodes, "required_steps_missing") {
		hours /= 2
	}
	due := record.RecordedAt.Add(time.Duration(hours) * time.Hour)
	if due.After(m.WindowStart) {
		due = m.WindowStart
	}
	return due.UTC()
}

func projectRemediationDeadlines(m *DiveMission, now time.Time) []RemediationDeadline {
	var out []RemediationDeadline
	drills, remediations, retests := latestRecords(m.Verifications, "drill"), latestRecords(m.Verifications, "remediation"), latestRecords(m.Verifications, "retest")
	for code, drill := range drills {
		if drill.Outcome != "deviation" {
			continue
		}
		status := "open"
		remaining := int64(drill.RemediationDueAt.Sub(now).Seconds())
		remediation := remediations[code]
		retest := retests[code]
		liveOverdue := remediation.ID == "" && now.After(drill.RemediationDueAt)
		if remediation.ID != "" {
			status = "awaiting_retest"
		}
		if retest.Outcome == "pass" && retest.RemediationCycle == remediation.RemediationCycle {
			status = "closed"
			remaining = 0
		}
		if remediation.ID == "" && liveOverdue {
			status = "overdue"
		}
		if remediation.ID == "" && !liveOverdue && remaining <= 3600 {
			status = "due_soon"
		}
		out = append(out, RemediationDeadline{CheckCode: code, DueAt: drill.RemediationDueAt, Status: status, RemainingSeconds: remaining, Overdue: remediation.WasOverdue || liveOverdue, DelaySeconds: remediation.DelaySeconds, DelayReason: remediation.DelayReason, DelayReviewedBy: remediation.DelayReviewedBy, RuleVersion: remediationRuleVersion})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DueAt.Equal(out[j].DueAt) {
			return out[i].CheckCode < out[j].CheckCode
		}
		return out[i].DueAt.Before(out[j].DueAt)
	})
	return out
}
