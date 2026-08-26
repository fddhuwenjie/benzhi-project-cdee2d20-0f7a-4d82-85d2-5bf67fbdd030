package mission

import (
	"regexp"
	"strings"
	"time"
)

var digestPattern = regexp.MustCompile(`^[a-fA-F0-9]{16,128}$`)

func ValidateMeta(meta CommandMeta) error {
	if strings.TrimSpace(meta.RequestID) == "" || len(meta.RequestID) > 128 {
		return Invalid("request_id", "必须提供且长度不超过 128")
	}
	if strings.TrimSpace(meta.ActorID) == "" || len(meta.ActorID) > 128 {
		return Invalid("actor_id", "必须提供且长度不超过 128")
	}
	if meta.ExpectedRevision < 0 {
		return Invalid("expected_revision", "不能为负数")
	}
	return nil
}

func ValidateCreate(in CreateInput, now time.Time) error {
	if err := ValidateMeta(in.Meta); err != nil {
		return err
	}
	if in.Meta.ExpectedRevision != 0 {
		return Invalid("expected_revision", "创建任务时必须为 0")
	}
	if strings.TrimSpace(in.Title) == "" || len(in.Title) > 160 {
		return Invalid("title", "必须提供且长度不超过 160")
	}
	if strings.TrimSpace(in.CaveSite) == "" || len(in.CaveSite) > 160 {
		return Invalid("cave_site", "必须提供且长度不超过 160")
	}
	if in.TargetDepthM <= 0 || in.TargetDepthM > 300 {
		return Invalid("target_depth_m", "必须在 0 到 300 米之间")
	}
	if in.WindowStart.IsZero() || in.WindowEnd.IsZero() || !in.WindowEnd.After(in.WindowStart) {
		return Invalid("window_end", "必须晚于 window_start")
	}
	if in.WindowEnd.Sub(in.WindowStart) > 72*time.Hour {
		return Invalid("window_end", "任务时间窗不能超过 72 小时")
	}
	if len(in.Segments) == 0 {
		return Invalid("segments", "至少需要一个洞段")
	}
	seenSegment := map[string]bool{}
	for _, segment := range in.Segments {
		segment = strings.TrimSpace(segment)
		if segment == "" || seenSegment[segment] {
			return Invalid("segments", "洞段名称必须非空且唯一")
		}
		seenSegment[segment] = true
	}
	if len(in.TeamMembers) < 3 {
		return Invalid("team_members", "至少需要领队、支援与待命人员")
	}
	roles, people := map[string]bool{}, map[string]bool{}
	for _, member := range in.TeamMembers {
		if strings.TrimSpace(member.PersonID) == "" || strings.TrimSpace(member.Name) == "" || strings.TrimSpace(member.Role) == "" {
			return Invalid("team_members", "成员 person_id、name 和 role 均为必填")
		}
		if people[member.PersonID] {
			return Invalid("team_members", "成员 person_id 必须唯一")
		}
		if roles[member.Role] {
			return Invalid("team_members", "任务成员职责必须唯一")
		}
		people[member.PersonID], roles[member.Role] = true, true
	}
	for _, role := range []string{"leader", "support", "standby"} {
		if !roles[role] {
			return Invalid("team_members", "必须包含 leader、support 和 standby 职责")
		}
	}
	_ = now
	return nil
}

func ValidateRisks(m *DiveMission, in RiskInput) ([]SegmentRisk, error) {
	preview, err := calculateRiskPreview(m, in.Risks)
	if err != nil {
		return nil, err
	}
	if !preview.Passed {
		return nil, Unprocessable("risks", "风险输入未通过门禁", map[string]any{"issues": preview.Issues, "risks": preview.Risks, "summary": preview.Summary})
	}
	return preview.Risks, nil
}

func SummarizeRisks(risks []SegmentRisk) RiskSummary {
	levels := map[string]int{"low": 0, "medium": 1, "high": 2, "critical": 3}
	summary := RiskSummary{Distribution: map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}}
	for _, risk := range risks {
		summary.TotalScore += risk.Score
		summary.Distribution[risk.RiskLevel]++
		if summary.HighestLevel == "" || levels[risk.RiskLevel] > levels[summary.HighestLevel] {
			summary.HighestLevel = risk.RiskLevel
		}
	}
	return summary
}

func ValidatePlan(m *DiveMission, in PlanInput) (GasCrossCheck, error) {
	if len(in.Members) < 3 {
		return GasCrossCheck{}, Invalid("members", "方案至少包含三名成员")
	}
	team := map[string]string{}
	for _, member := range m.TeamMembers {
		team[member.PersonID] = member.Role
	}
	seen := map[string]bool{}
	for _, member := range in.Members {
		if team[member.PersonID] == "" || team[member.PersonID] != member.Role {
			return GasCrossCheck{}, Invalid("members", "方案成员及职责必须与任务草案一致")
		}
		if seen[member.PersonID] {
			return GasCrossCheck{}, Invalid("members", "方案成员不能重复")
		}
		seen[member.PersonID] = true
	}
	if len(in.GasMixes) < 2 {
		return GasCrossCheck{}, Invalid("gas_mixes", "至少需要主用与冗余两套呼吸气体")
	}
	gasNames := map[string]bool{}
	for _, mix := range in.GasMixes {
		name := strings.ToLower(strings.TrimSpace(mix.Name))
		if gasNames[name] {
			return GasCrossCheck{}, Unprocessable("gas_mixes", "气体名称重复", map[string]any{"gas_name": mix.Name})
		}
		gasNames[name] = true
		if name == "" || mix.OxygenPercent < 10 || mix.OxygenPercent > 40 || mix.HeliumPercent < 0 || mix.HeliumPercent > 90 || mix.OxygenPercent+mix.HeliumPercent > 100 {
			return GasCrossCheck{}, Unprocessable("gas_mixes", "气体名称或氧氦配比无效", map[string]any{"gas_name": mix.Name})
		}
		if mix.StartPressureBar < 150 || mix.StartPressureBar > 350 || mix.CylinderLiters <= 0 {
			return GasCrossCheck{}, Unprocessable("gas_mixes", "起始压力或气瓶容量无效", map[string]any{"gas_name": mix.Name})
		}
		if mix.IntendedDepthM <= 0 || mix.IntendedDepthM > 300 {
			return GasCrossCheck{}, Unprocessable("intended_depth_m", "气体声明使用深度无效", map[string]any{"gas_name": mix.Name, "intended_depth_m": mix.IntendedDepthM})
		}
	}
	if in.TurnPressureBar < 50 || in.TurnPressureBar >= in.GasMixes[0].StartPressureBar {
		return GasCrossCheck{}, Invalid("turn_pressure_bar", "必须保留有效的转向压力余量")
	}
	if in.ReserveRule != "rule_of_thirds" && in.ReserveRule != "minimum_gas" {
		return GasCrossCheck{}, Invalid("reserve_rule", "仅支持 rule_of_thirds 或 minimum_gas")
	}
	duties, assigned := map[string]bool{}, map[string]bool{}
	for _, a := range in.SupportAssignments {
		if team[a.PersonID] == "" || strings.TrimSpace(a.Duty) == "" {
			return GasCrossCheck{}, Invalid("support_assignments", "支援指派必须引用任务成员并填写职责")
		}
		if duties[a.Duty] || assigned[a.PersonID] {
			return GasCrossCheck{}, Unprocessable("support_assignments", "支援与待命职责及人员必须唯一", map[string]any{"person_id": a.PersonID, "duty": a.Duty})
		}
		duties[a.Duty], assigned[a.PersonID] = true, true
	}
	if !duties["surface_support"] || !duties["standby_diver"] {
		return GasCrossCheck{}, Invalid("support_assignments", "必须指派 surface_support 和 standby_diver")
	}
	depthPressure := int(m.TargetDepthM/10) + 1
	cross := GasCrossCheck{Passed: true, TargetDepthBar: depthPressure}
	for index, mix := range in.GasMixes {
		reserve := mix.StartPressureBar / 3
		if in.ReserveRule == "minimum_gas" {
			reserve = depthPressure * 10
		}
		available := mix.StartPressureBar - in.TurnPressureBar
		margin := GasMargin{GasName: mix.Name, Role: "redundant", AvailableBar: available, RequiredReserveBar: reserve, MarginBar: available - reserve, Passed: available >= reserve && mix.StartPressureBar >= in.TurnPressureBar+depthPressure*5}
		if index == 0 {
			margin.Role = "primary"
		}
		cross.Margins = append(cross.Margins, margin)
		if !margin.Passed {
			cross.Passed = false
			return cross, Unprocessable("gas_mixes", "气体余量不足以覆盖目标深度、转向压力与储备规则", map[string]any{"gas_name": mix.Name, "margin": margin})
		}
		adaptation := CalculateDepthAdaptation(m, mix)
		if index == 0 && adaptation.IntendedDepthM+0.01 < m.TargetDepthM {
			return cross, Unprocessable("gas_mixes", "主用气体声明深度不能覆盖任务目标深度", map[string]any{"gas_name": mix.Name, "calculated": adaptation, "required_depth_m": m.TargetDepthM})
		}
		if !adaptation.Passed {
			return cross, Unprocessable("gas_mixes", "呼吸气体不满足目标使用深度的氧分压、最大操作深度或等效麻醉深度门禁", map[string]any{"gas_name": mix.Name, "calculated": adaptation, "oxygen_partial_pressure_range_bar": []float64{0.16, 1.4}, "maximum_equivalent_narcotic_depth_m": 40.0})
		}
	}
	return cross, nil
}

func validEvidence(value string) bool { return digestPattern.MatchString(value) }

func memberIDs(m *DiveMission) map[string]bool {
	result := map[string]bool{}
	for _, member := range m.TeamMembers {
		result[member.PersonID] = true
	}
	return result
}
