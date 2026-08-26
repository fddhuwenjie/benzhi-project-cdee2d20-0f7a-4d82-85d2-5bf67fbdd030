package mission

import (
	"sort"
	"strings"
)

func CalculateMemberGasMargins(m *DiveMission, assignments []MemberGasAssignment, mixes []GasMix, turnPressureBar int) ([]MemberGasMargin, error) {
	if len(assignments) == 0 {
		return nil, nil
	}
	members := map[string]bool{}
	for _, member := range m.TeamMembers {
		members[member.PersonID] = true
	}
	mixByName := map[string]GasMix{}
	for _, mix := range mixes {
		mixByName[strings.ToLower(strings.TrimSpace(mix.Name))] = mix
	}
	longestExit := 0
	for _, risk := range m.Risks {
		if risk.ExitLimitMin > longestExit {
			longestExit = risk.ExitLimitMin
		}
	}
	seenMembers, assets := map[string]bool{}, map[string]string{}
	margins := make([]MemberGasMargin, 0, len(assignments)*2)
	for _, assignment := range assignments {
		if assignment.PrimaryAssetID == "" {
			assignment.PrimaryAssetID, assignment.PrimaryGasMix = assignment.Primary.AssetID, assignment.Primary.GasMix
		}
		if assignment.RedundantAssetID == "" {
			assignment.RedundantAssetID, assignment.RedundantGasMix = assignment.Redundant.AssetID, assignment.Redundant.GasMix
		}
		if !members[assignment.PersonID] || seenMembers[assignment.PersonID] {
			return nil, Unprocessable("member_gas_assignments", "成员气体分配必须逐人唯一并引用任务成员", map[string]any{"person_id": assignment.PersonID})
		}
		seenMembers[assignment.PersonID] = true
		if assignment.SurfaceConsumptionLMin <= 0 || assignment.SurfaceConsumptionLMin > 100 {
			return nil, Unprocessable("surface_consumption_l_min", "成员水面耗气率必须在 0 到 100 升/分钟之间", map[string]any{"person_id": assignment.PersonID})
		}
		primaryAsset := strings.TrimSpace(assignment.PrimaryAssetID)
		redundantAsset := strings.TrimSpace(assignment.RedundantAssetID)
		if primaryAsset == "" || redundantAsset == "" || primaryAsset == redundantAsset {
			return nil, Unprocessable("asset_id", "每名成员必须绑定不同的主用和冗余气瓶", map[string]any{"person_id": assignment.PersonID, "primary_asset_id": primaryAsset, "redundant_asset_id": redundantAsset})
		}
		for role, source := range map[string]struct{ asset, gas string }{"primary": {primaryAsset, assignment.PrimaryGasMix}, "redundant": {redundantAsset, assignment.RedundantGasMix}} {
			if owner, used := assets[source.asset]; used {
				return nil, Unprocessable("asset_id", "气瓶资产在成员分配矩阵中必须唯一", map[string]any{"person_id": assignment.PersonID, "source_role": role, "asset_id": source.asset, "used_by": owner})
			}
			assets[source.asset] = assignment.PersonID + ":" + role
			mix, ok := mixByName[strings.ToLower(strings.TrimSpace(source.gas))]
			if !ok {
				return nil, Unprocessable("gas_mix", "成员气源必须引用方案内唯一命名的 gas_mix", map[string]any{"person_id": assignment.PersonID, "source_role": role, "gas_mix": source.gas})
			}
			adaptation := CalculateDepthAdaptation(m, GasMix{Name: mix.Name, OxygenPercent: mix.OxygenPercent, HeliumPercent: mix.HeliumPercent, StartPressureBar: mix.StartPressureBar, CylinderLiters: mix.CylinderLiters, IntendedDepthM: m.TargetDepthM})
			if !adaptation.Passed {
				return nil, Unprocessable("member_gas_assignments", "成员气源不满足目标深度适配门禁", map[string]any{"person_id": assignment.PersonID, "source_role": role, "calculated": adaptation})
			}
			available := round2(float64(mix.StartPressureBar-turnPressureBar) * mix.CylinderLiters)
			required := round2(assignment.SurfaceConsumptionLMin * (1 + m.TargetDepthM/10) * float64(longestExit))
			margin := MemberGasMargin{PersonID: assignment.PersonID, SourceRole: role, AssetID: source.asset, GasMix: mix.Name, AvailableLiters: available, RequiredLiters: required, MarginLiters: round2(available - required), Passed: available >= required}
			margins = append(margins, margin)
		}
	}
	if len(seenMembers) != len(members) {
		return nil, Unprocessable("member_gas_assignments", "每名任务成员都必须具有主用与冗余气源", map[string]any{"required_members": len(members), "provided_members": len(seenMembers)})
	}
	sort.Slice(margins, func(i, j int) bool {
		if margins[i].PersonID == margins[j].PersonID {
			return margins[i].SourceRole < margins[j].SourceRole
		}
		return margins[i].PersonID < margins[j].PersonID
	})
	failed := []MemberGasMargin{}
	for _, margin := range margins {
		if !margin.Passed {
			failed = append(failed, margin)
		}
	}
	if len(failed) > 0 {
		return margins, Unprocessable("member_gas_assignments", "成员气源可用量不足以覆盖最长撤离需求", map[string]any{"margins": margins, "failed": failed, "longest_exit_limit_min": longestExit})
	}
	return margins, nil
}
