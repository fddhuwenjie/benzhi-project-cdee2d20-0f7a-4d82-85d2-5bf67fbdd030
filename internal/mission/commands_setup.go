package mission

import (
	"context"
	"sort"
	"strings"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

func canonicalSiteKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func (s *Service) Create(ctx context.Context, in CreateInput) (StoredResult, bool, error) {
	now := s.now().UTC()
	if err := ValidateMeta(in.Meta); err != nil {
		return StoredResult{}, false, err
	}
	if in.Meta.ExpectedRevision != 0 {
		return StoredResult{}, false, Invalid("expected_revision", "创建任务时必须为 0")
	}
	missionID := newID("mission")
	fingerprint, err := commandFingerprint("create_mission", in.Meta, in)
	if err != nil {
		return StoredResult{}, false, err
	}
	return s.repo.Execute(ctx, missionID, in.Meta.RequestID, "create_mission", fingerprint, func(tx Tx) (StoredResult, error) {
		resolved := in
		var templateDigest string
		if strings.TrimSpace(in.TemplateMissionID) != "" {
			template, loadErr := tx.LoadMission(ctx, strings.TrimSpace(in.TemplateMissionID))
			if loadErr != nil {
				return StoredResult{}, NewError("template_invalid", "归档模板不存在", 422)
			}
			if template.Status != StatusArchived {
				return StoredResult{}, NewError("template_not_archived", "模板任务必须已经归档", 422)
			}
			events, eventsErr := tx.AllEvents(ctx, template.ID)
			if eventsErr != nil || audit.Verify(events) != nil || len(events) < 2 || events[len(events)-1].EventType != "mission_archived" || events[len(events)-2].EventHash != template.ArchiveDigest {
				return StoredResult{}, NewError("template_integrity_failed", "模板归档审计链或归档摘要校验失败", 409)
			}
			templateDigest = template.ArchiveDigest
			resolved.CaveSite, resolved.TargetDepthM = template.CaveSite, template.TargetDepthM
			resolved.Segments = append([]string(nil), template.Segments...)
			resolved.TeamMembers = append([]Member(nil), template.TeamMembers...)
			if strings.TrimSpace(resolved.Title) == "" {
				resolved.Title = template.Title
			}
			if len(in.TeamMembers) > 0 {
				names := map[string]string{}
				for _, member := range in.TeamMembers {
					known := false
					for _, original := range template.TeamMembers {
						if original.PersonID == member.PersonID {
							known = true
							break
						}
					}
					if !known {
						return StoredResult{}, Unprocessable("team_members", "模板派生只能覆盖模板成员姓名", map[string]any{"person_id": member.PersonID})
					}
					names[member.PersonID] = strings.TrimSpace(member.Name)
				}
				for i := range resolved.TeamMembers {
					if name, ok := names[resolved.TeamMembers[i].PersonID]; ok && name != "" {
						resolved.TeamMembers[i].Name = name
					}
				}
			}
		}
		if err := ValidateCreate(resolved, now); err != nil {
			return StoredResult{}, err
		}
		var qualifications []MemberQualification
		var qualificationStatus []QualificationStatus
		if len(resolved.MemberQualifications) > 0 {
			qualifications, qualificationStatus, err = ValidateQualifications(resolved.TeamMembers, resolved.MemberQualifications, resolved.TargetDepthM, resolved.WindowEnd)
			if err != nil {
				return StoredResult{}, err
			}
		}
		siteKey := canonicalSiteKey(resolved.CaveSite)
		conflicts, err := tx.ScheduleConflicts(ctx, siteKey, resolved.WindowStart.UTC(), resolved.WindowEnd.UTC())
		if err != nil {
			return StoredResult{}, err
		}
		currentDigest, digestErr := scheduleDigest(siteKey, resolved.WindowStart, resolved.WindowEnd, conflicts)
		if digestErr != nil {
			return StoredResult{}, digestErr
		}
		if len(conflicts) > 0 || resolved.SourceDigest != "" && resolved.SourceDigest != currentDigest {
			return StoredResult{}, &Error{Code: "schedule_conflict", Message: "任务时间窗与同站点未归档任务重叠", Status: 409, Details: map[string]any{"conflicts": conflicts}}
		}
		leaderID := ""
		members := append([]Member(nil), resolved.TeamMembers...)
		for _, member := range members {
			if member.Role == "leader" {
				leaderID = member.PersonID
			}
		}
		segments := make([]string, len(resolved.Segments))
		for i, segment := range resolved.Segments {
			segments[i] = strings.TrimSpace(segment)
		}
		m := &DiveMission{ID: missionID, Title: strings.TrimSpace(resolved.Title), CaveSite: strings.TrimSpace(resolved.CaveSite), CaveSiteKey: siteKey,
			TargetDepthM: resolved.TargetDepthM, WindowStart: resolved.WindowStart.UTC(), WindowEnd: resolved.WindowEnd.UTC(),
			Status: StatusDraft, Revision: 1, LeaderID: leaderID, Segments: segments, TeamMembers: members,
			MemberQualifications: qualifications, QualificationStatus: qualificationStatus,
			Risks: []SegmentRisk{}, Verifications: []VerificationRecord{}, ScheduleCheck: ScheduleCheck{Checked: true, CaveSiteKey: siteKey, Conflicts: []ScheduleConflict{}}, TemplateMissionID: strings.TrimSpace(in.TemplateMissionID), TemplateArchiveDigest: templateDigest, CreatedAt: now, UpdatedAt: now}
		if err := tx.InsertMission(ctx, m); err != nil {
			return StoredResult{}, err
		}
		eventPayload := struct {
			Draft                    *DiveMission `json:"draft"`
			TemplateMissionID        string       `json:"template_mission_id,omitempty"`
			TemplateArchiveDigest    string       `json:"template_archive_digest,omitempty"`
			QualificationRuleVersion string       `json:"qualification_rule_version,omitempty"`
		}{m, m.TemplateMissionID, m.TemplateArchiveDigest, qualificationRuleVersion}
		event, err := audit.Build(m.ID, "mission_created", in.Meta.ActorID, in.Meta.RequestID, 0, 1, eventPayload, now)
		if err != nil {
			return StoredResult{}, err
		}
		event.StatusAfter = string(m.Status)
		if _, err = tx.AppendEvent(ctx, event); err != nil {
			return StoredResult{}, err
		}
		return resultFor(m, 201)
	})
}

func (s *Service) SubmitRisks(ctx context.Context, id string, in RiskInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "submit_risks", in.Meta, in, func(_ context.Context, _ Tx, m *DiveMission) (string, any, error) {
		if m.Status != StatusDraft {
			return "", nil, InvalidState(m.Status, "风险评估")
		}
		preview, err := calculateRiskPreview(m, in.Risks)
		if err != nil {
			return "", nil, err
		}
		if in.RiskPreviewDigest != "" && in.RiskPreviewDigest != preview.RiskPreviewDigest {
			return "", nil, &Error{Code: "preview_conflict", Message: "风险试算摘要与当前草案修订不一致", Status: 409, Details: map[string]any{"source_revision": m.Revision, "rule_version": riskRuleVersion}}
		}
		risks, err := ValidateRisks(m, in)
		if err != nil {
			return "", nil, err
		}
		for i := range risks {
			risks[i].ID = newID("risk")
			risks[i].MissionID = m.ID
			risks[i].AssessedBy = in.Meta.ActorID
		}
		if hasStructuredMitigations(risks) {
			risks, err = validateMitigationActions(m, risks)
			if err != nil {
				return "", nil, err
			}
		}
		m.Risks, m.RiskSummary, m.Status = risks, SummarizeRisks(risks), StatusRiskAssessed
		return "risks_assessed", struct {
			Risks             []SegmentRisk `json:"risks"`
			Summary           RiskSummary   `json:"summary"`
			RuleVersion       string        `json:"rule_version"`
			RiskPreviewDigest string        `json:"risk_preview_digest"`
		}{risks, m.RiskSummary, riskRuleVersion, preview.RiskPreviewDigest}, nil
	})
}

func (s *Service) SubmitPlan(ctx context.Context, id string, in PlanInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "submit_life_support_plan", in.Meta, in, func(_ context.Context, _ Tx, m *DiveMission) (string, any, error) {
		if m.Status != StatusRiskAssessed {
			return "", nil, InvalidState(m.Status, "提交生命支持方案")
		}
		if blockers := mitigationBlockers(m, s.now().UTC()); len(blockers) > 0 {
			return "", nil, &Error{Code: "risk_mitigation_incomplete", Message: "风险缓解行动尚未全部完成", Status: 409, Details: map[string]any{"blockers": blockers}}
		}
		previous := m.LifeSupportPlan
		if previous != nil && previous.ReviewStatus == "rejected" {
			if in.RevisedFromPlanID == "" || in.RevisedFromPlanID != m.LifeSupportPlan.ID {
				return "", nil, Unprocessable("revised_from_plan_id", "重新提交必须引用最近退回的方案版本", map[string]any{"latest_rejected_plan_id": m.LifeSupportPlan.ID})
			}
			m.PlanHistory = append(m.PlanHistory, LifeSupportPlanVersion{Plan: *m.LifeSupportPlan, Version: int64(len(m.PlanHistory) + 1), RejectedBy: m.LifeSupportPlan.ReviewedBy, Reason: m.LifeSupportPlan.ReviewNote, Revision: m.Revision - 1})
		}
		crossCheck, err := ValidatePlan(m, in)
		if err != nil {
			return "", nil, err
		}
		memberMargins, err := CalculateMemberGasMargins(m, in.MemberGasAssignments, in.GasMixes, in.TurnPressureBar)
		if err != nil {
			return "", nil, err
		}
		scenarios, err := calculateSegmentGasScenarios(m, in)
		if err != nil {
			return "", nil, err
		}
		adaptations := make([]GasDepthAdaptation, 0, len(in.GasMixes))
		for _, mix := range in.GasMixes {
			adaptations = append(adaptations, CalculateDepthAdaptation(m, mix))
		}
		sort.Slice(adaptations, func(i, j int) bool { return adaptations[i].GasName < adaptations[j].GasName })
		m.LifeSupportPlan = &LifeSupportPlan{ID: newID("plan"), Version: int64(len(m.PlanHistory) + 1), MissionID: m.ID, Members: in.Members, GasMixes: in.GasMixes,
			TurnPressureBar: in.TurnPressureBar, ReserveRule: in.ReserveRule, SupportAssignments: in.SupportAssignments, ReviewStatus: "pending", CrossCheck: crossCheck}
		m.LifeSupportPlan.DepthAdaptations = adaptations
		m.LifeSupportPlan.RevisedFromPlanID = in.RevisedFromPlanID
		m.LifeSupportPlan.RemediationNotes = in.RemediationNotes
		m.LifeSupportPlan.MemberGasAssignments = append([]MemberGasAssignment(nil), in.MemberGasAssignments...)
		m.LifeSupportPlan.MemberGasMargins = memberMargins
		m.LifeSupportPlan.SegmentGasBudgets = append([]SegmentGasBudget(nil), in.SegmentGasBudgets...)
		m.LifeSupportPlan.ScenarioMargins = scenarios
		m.LifeSupportPlan.BudgetRuleVersion = gasBudgetRuleVersion
		m.LifeSupportPlan.RevisionDiff = planDiff(previous, in)
		m.Status = StatusPlanReview
		eventType := "life_support_plan_submitted"
		if previous != nil {
			eventType = "life_support_plan_revised"
		}
		return eventType, m.LifeSupportPlan, nil
	})
}

func (s *Service) ReviewPlan(ctx context.Context, id string, in ReviewInput) (StoredResult, bool, error) {
	return s.command(ctx, id, "review_life_support_plan", in.Meta, in, func(_ context.Context, _ Tx, m *DiveMission) (string, any, error) {
		if m.Status != StatusPlanReview || m.LifeSupportPlan == nil {
			return "", nil, InvalidState(m.Status, "审核生命支持方案")
		}
		if in.Meta.ActorID == m.LeaderID || memberIDs(m)[in.Meta.ActorID] {
			return "", nil, Invalid("actor_id", "审核员必须独立于任务团队")
		}
		if in.Decision != "approve" && in.Decision != "reject" {
			return "", nil, Invalid("decision", "必须为 approve 或 reject")
		}
		if in.Decision == "reject" && strings.TrimSpace(in.Reason) == "" {
			return "", nil, Invalid("reason", "退回时必须填写理由")
		}
		m.LifeSupportPlan.ReviewedBy, m.LifeSupportPlan.ReviewNote = in.Meta.ActorID, strings.TrimSpace(in.Reason)
		if in.Decision == "approve" {
			if len(m.PlanHistory) > 0 {
				latest := m.PlanHistory[len(m.PlanHistory)-1].Plan
				for _, rule := range latest.FailedRules {
					if strings.TrimSpace(m.LifeSupportPlan.RemediationNotes[rule]) == "" {
						return "", nil, Unprocessable("remediation_notes", "整改说明未覆盖最近退回问题", map[string]any{"failed_rule": rule})
					}
				}
			}
			if !m.LifeSupportPlan.CrossCheck.Passed {
				return "", nil, Unprocessable("life_support_plan", "交叉核验未通过", m.LifeSupportPlan.CrossCheck)
			}
			memberMargins, marginErr := CalculateMemberGasMargins(m, m.LifeSupportPlan.MemberGasAssignments, m.LifeSupportPlan.GasMixes, m.LifeSupportPlan.TurnPressureBar)
			if marginErr != nil {
				return "", nil, marginErr
			}
			m.LifeSupportPlan.MemberGasMargins = memberMargins
			recheck := PlanInput{Members: m.LifeSupportPlan.Members, GasMixes: m.LifeSupportPlan.GasMixes, TurnPressureBar: m.LifeSupportPlan.TurnPressureBar, ReserveRule: m.LifeSupportPlan.ReserveRule, MemberGasAssignments: m.LifeSupportPlan.MemberGasAssignments, SegmentGasBudgets: m.LifeSupportPlan.SegmentGasBudgets}
			scenarios, scenarioErr := calculateSegmentGasScenarios(m, recheck)
			if scenarioErr != nil {
				return "", nil, scenarioErr
			}
			m.LifeSupportPlan.ScenarioMargins = scenarios
			adaptations := make([]GasDepthAdaptation, 0, len(m.LifeSupportPlan.GasMixes))
			for _, mix := range m.LifeSupportPlan.GasMixes {
				a := CalculateDepthAdaptation(m, mix)
				if !a.Passed {
					return "", nil, Unprocessable("gas_mixes", "审核复算发现气体深度适配失败", map[string]any{"gas_name": mix.Name, "calculated": a})
				}
				adaptations = append(adaptations, a)
			}
			sort.Slice(adaptations, func(i, j int) bool { return adaptations[i].GasName < adaptations[j].GasName })
			m.LifeSupportPlan.DepthAdaptations = adaptations
			m.LifeSupportPlan.ReviewStatus, m.Status = "approved", StatusEquipmentVerification
			return "life_support_plan_approved", struct {
				Review           ReviewInput          `json:"review"`
				PlanID           string               `json:"plan_id"`
				PlanVersion      int64                `json:"plan_version"`
				CrossCheck       GasCrossCheck        `json:"cross_check"`
				DepthAdaptations []GasDepthAdaptation `json:"depth_adaptations"`
				MemberGasMargins []MemberGasMargin    `json:"member_gas_margins"`
				ScenarioMargins  []GasFailureScenario `json:"scenario_margins"`
				FailedRules      []string             `json:"failed_rules"`
			}{in, m.LifeSupportPlan.ID, m.LifeSupportPlan.Version, m.LifeSupportPlan.CrossCheck, m.LifeSupportPlan.DepthAdaptations, m.LifeSupportPlan.MemberGasMargins, m.LifeSupportPlan.ScenarioMargins, m.LifeSupportPlan.FailedRules}, nil
		}
		failedRules := make([]string, 0, len(in.FailedRules))
		seen := map[string]bool{}
		for _, rule := range in.FailedRules {
			rule = strings.TrimSpace(rule)
			if rule == "" || seen[rule] {
				return "", nil, Invalid("failed_rules", "失败规则必须非空且唯一")
			}
			seen[rule] = true
			failedRules = append(failedRules, rule)
		}
		if len(failedRules) == 0 {
			failedRules = []string{strings.TrimSpace(in.Reason)}
		}
		sort.Strings(failedRules)
		m.LifeSupportPlan.ReviewStatus, m.LifeSupportPlan.FailedRules, m.Status = "rejected", failedRules, StatusRiskAssessed
		return "life_support_plan_rejected", map[string]any{"review": in, "plan_id": m.LifeSupportPlan.ID, "plan_version": m.LifeSupportPlan.Version, "failed_rules": m.LifeSupportPlan.FailedRules}, nil
	})
}
