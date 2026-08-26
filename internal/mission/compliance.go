package mission

import (
	"sort"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

func actorSequence(events []audit.Event, actor string) []int64 {
	out := []int64{}
	for _, event := range events {
		if event.ActorID == actor {
			out = append(out, event.Sequence)
		}
	}
	return out
}

func complianceSummary(m *DiveMission, events []audit.Event) (ComplianceSummary, error) {
	summary := ComplianceSummary{HighestRisk: m.RiskSummary.HighestLevel, EquipmentReplacementLineage: []EvidenceReference{}, DrillOverruns: []map[string]any{}, FinalResolutionCycles: []map[string]any{}, OverdueRemediations: []map[string]any{}}
	first := map[Status]time.Time{}
	for _, event := range events {
		status := Status(event.StatusAfter)
		if first[status].IsZero() {
			first[status] = event.OccurredAt
		}
		switch event.EventType {
		case "life_support_plan_rejected":
			summary.PlanRejectionCount++
		case "equipment_replaced":
			summary.EquipmentReplacementCount++
		}
	}
	orderedStatuses := []Status{StatusDraft, StatusRiskAssessed, StatusPlanReview, StatusEquipmentVerification, StatusDrillPending, StatusRemediation, StatusReadyForRelease, StatusReleaseRejected, StatusSigned, StatusArchived}
	entered := []StageTiming{}
	for _, status := range orderedStatuses {
		if !first[status].IsZero() {
			entered = append(entered, StageTiming{Status: status, FirstEnteredAt: first[status]})
		}
	}
	sort.Slice(entered, func(i, j int) bool { return entered[i].FirstEnteredAt.Before(entered[j].FirstEnteredAt) })
	for i := range entered {
		end := events[len(events)-1].OccurredAt
		if i+1 < len(entered) {
			end = entered[i+1].FirstEnteredAt
		}
		entered[i].DurationSeconds = int64(end.Sub(entered[i].FirstEnteredAt).Seconds())
	}
	summary.StageTimings = entered
	if len(events) > 0 {
		summary.TotalDurationSeconds = int64(events[len(events)-1].OccurredAt.Sub(events[0].OccurredAt).Seconds())
	}
	minimumSet := false
	if m.LifeSupportPlan != nil {
		for _, margin := range m.LifeSupportPlan.MemberGasMargins {
			if !minimumSet || margin.MarginLiters < summary.MinimumGasMarginLiters {
				summary.MinimumGasMarginLiters, minimumSet = margin.MarginLiters, true
			}
		}
		for _, margin := range m.LifeSupportPlan.ScenarioMargins {
			if !minimumSet || margin.MarginLiters < summary.MinimumGasMarginLiters {
				summary.MinimumGasMarginLiters, minimumSet = margin.MarginLiters, true
			}
		}
	}
	allRecords := append(append([]VerificationRecord(nil), m.VerificationHistory...), m.Verifications...)
	maxCycles := map[string]int{}
	for _, record := range allRecords {
		if record.RecordType == "drill" && record.Outcome == "deviation" {
			summary.DrillDeviationCount++
		}
		if record.RecordType == "drill" && record.DurationSeconds > record.RequiredMaxDurationSeconds && record.RequiredMaxDurationSeconds > 0 {
			summary.DrillOverruns = append(summary.DrillOverruns, map[string]any{"check_code": record.CheckCode, "observed_duration_seconds": record.DurationSeconds, "required_max_duration_seconds": record.RequiredMaxDurationSeconds, "exceeded_seconds": record.DurationSeconds - record.RequiredMaxDurationSeconds, "record_id": record.ID})
		}
		if record.RecordType == "retest" && record.Outcome == "fail" {
			summary.RetestFailureCount++
		}
		if record.RecordType == "remediation" && record.RemediationCycle > maxCycles[record.CheckCode] {
			maxCycles[record.CheckCode] = record.RemediationCycle
		}
		if record.RecordType == "remediation" && record.WasOverdue {
			summary.OverdueRemediations = append(summary.OverdueRemediations, map[string]any{"check_code": record.CheckCode, "record_id": record.ID, "due_at": record.RemediationDueAt, "delay_seconds": record.DelaySeconds, "delay_reason": record.DelayReason, "delay_reviewed_by": record.DelayReviewedBy})
		}
		if record.RecordType == "equipment" && record.ReplacementForAssetID != "" {
			ref, err := evidenceReference(events, "equipment", record.CheckCode, record.ID, record.VerifiedBy, record.RecordedAt, "replacement")
			if err != nil {
				return ComplianceSummary{}, err
			}
			summary.EquipmentReplacementLineage = append(summary.EquipmentReplacementLineage, ref)
		}
	}
	for code, cycle := range maxCycles {
		summary.RemediationCycleCount += cycle
		status := "unresolved"
		if latest, ok := latestCycleRecord(m, "retest", code); ok && latest.Outcome == "pass" {
			status = "passed"
		}
		summary.FinalResolutionCycles = append(summary.FinalResolutionCycles, map[string]any{"check_code": code, "final_cycle": cycle, "status": status})
	}
	sort.Slice(summary.DrillOverruns, func(i, j int) bool {
		return summary.DrillOverruns[i]["check_code"].(string) < summary.DrillOverruns[j]["check_code"].(string)
	})
	sort.Slice(summary.FinalResolutionCycles, func(i, j int) bool {
		return summary.FinalResolutionCycles[i]["check_code"].(string) < summary.FinalResolutionCycles[j]["check_code"].(string)
	})
	sort.Slice(summary.OverdueRemediations, func(i, j int) bool {
		return summary.OverdueRemediations[i]["check_code"].(string) < summary.OverdueRemediations[j]["check_code"].(string)
	})
	memberSet := memberIDs(m)
	addCheck := func(code string, actors []string, sequences []int64, missing bool) {
		status := "satisfied"
		if missing {
			status = "data_missing"
		} else {
			seen := map[string]bool{}
			for _, actor := range actors {
				if actor == "" {
					status = "data_missing"
					continue
				}
				if seen[actor] {
					status = "conflict"
				}
				seen[actor] = true
			}
		}
		sort.Strings(actors)
		sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
		summary.RoleIsolation = append(summary.RoleIsolation, RoleIsolationCheck{code, status, actors, sequences})
	}
	if m.LifeSupportPlan != nil {
		reviewer := m.LifeSupportPlan.ReviewedBy
		statusMissing := reviewer == ""
		actors := []string{reviewer}
		if memberSet[reviewer] {
			actors = append(actors, reviewer)
		}
		addCheck("plan_reviewer_vs_team", actors, actorSequence(events, reviewer), statusMissing)
	} else {
		addCheck("plan_reviewer_vs_team", nil, nil, true)
	}
	roleActors := map[string]map[string]bool{"equipment_verifier_vs_team_and_reviewer": {}, "drill_witness_vs_prior_roles": {}, "remediation_actor_vs_drill_and_reviewer": {}, "retest_actor_vs_cycle_roles": {}}
	for _, record := range allRecords {
		code := map[string]string{"equipment": "equipment_verifier_vs_team_and_reviewer", "drill": "drill_witness_vs_prior_roles", "remediation": "remediation_actor_vs_drill_and_reviewer", "retest": "retest_actor_vs_cycle_roles"}[record.RecordType]
		if code != "" {
			roleActors[code][record.VerifiedBy] = true
		}
	}
	for _, code := range []string{"equipment_verifier_vs_team_and_reviewer", "drill_witness_vs_prior_roles", "remediation_actor_vs_drill_and_reviewer", "retest_actor_vs_cycle_roles"} {
		actors, sequences := []string{}, []int64{}
		for actor := range roleActors[code] {
			actors = append(actors, actor)
			sequences = append(sequences, actorSequence(events, actor)...)
		}
		addCheck(code, actors, sequences, len(actors) == 0)
	}
	signerActors := []string{m.SignedBy}
	addCheck("signer_vs_all_operational_roles", signerActors, actorSequence(events, m.SignedBy), m.SignedBy == "")
	// Re-evaluate cross-role conflicts against the complete actor matrix.
	roleActorsByCode := map[string]map[string]bool{}
	for _, check := range summary.RoleIsolation {
		set := map[string]bool{}
		for _, actor := range check.ActorIDs {
			set[actor] = true
		}
		roleActorsByCode[check.CheckCode] = set
	}
	markConflict := func(code string, forbidden map[string]bool) {
		for i := range summary.RoleIsolation {
			if summary.RoleIsolation[i].CheckCode != code || summary.RoleIsolation[i].Status == "data_missing" {
				continue
			}
			for _, actor := range summary.RoleIsolation[i].ActorIDs {
				if forbidden[actor] {
					summary.RoleIsolation[i].Status = "conflict"
					break
				}
			}
		}
	}
	teamAndReviewer := map[string]bool{}
	for actor := range memberSet {
		teamAndReviewer[actor] = true
	}
	if m.LifeSupportPlan != nil {
		teamAndReviewer[m.LifeSupportPlan.ReviewedBy] = true
	}
	markConflict("plan_reviewer_vs_team", memberSet)
	markConflict("equipment_verifier_vs_team_and_reviewer", teamAndReviewer)
	prior := map[string]bool{}
	for actor := range teamAndReviewer {
		prior[actor] = true
	}
	for actor := range roleActorsByCode["equipment_verifier_vs_team_and_reviewer"] {
		prior[actor] = true
	}
	markConflict("drill_witness_vs_prior_roles", prior)
	remediationForbidden := map[string]bool{}
	for actor := range roleActorsByCode["drill_witness_vs_prior_roles"] {
		remediationForbidden[actor] = true
	}
	if m.LifeSupportPlan != nil {
		remediationForbidden[m.LifeSupportPlan.ReviewedBy] = true
	}
	markConflict("remediation_actor_vs_drill_and_reviewer", remediationForbidden)
	retestForbidden := map[string]bool{}
	for actor := range remediationForbidden {
		retestForbidden[actor] = true
	}
	markConflict("retest_actor_vs_cycle_roles", retestForbidden)
	operational := map[string]bool{}
	for actor := range teamAndReviewer {
		operational[actor] = true
	}
	for _, code := range []string{"equipment_verifier_vs_team_and_reviewer", "drill_witness_vs_prior_roles", "remediation_actor_vs_drill_and_reviewer", "retest_actor_vs_cycle_roles"} {
		for actor := range roleActorsByCode[code] {
			operational[actor] = true
		}
	}
	markConflict("signer_vs_all_operational_roles", operational)
	sort.Slice(summary.RoleIsolation, func(i, j int) bool { return summary.RoleIsolation[i].CheckCode < summary.RoleIsolation[j].CheckCode })
	return summary, nil
}
