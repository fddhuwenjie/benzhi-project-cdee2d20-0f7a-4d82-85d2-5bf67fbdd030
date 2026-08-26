package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func saveSegmentsAndMembers(ctx context.Context, tx *sql.Tx, m *mission.DiveMission) error {
	for position, segment := range m.Segments {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mission_segments(mission_id, position, name) VALUES (?, ?, ?)`, m.ID, position, segment); err != nil {
			return err
		}
	}
	for _, member := range m.TeamMembers {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mission_members(mission_id, person_id, name, role) VALUES (?, ?, ?, ?)`, m.ID, member.PersonID, member.Name, member.Role); err != nil {
			return err
		}
	}
	return nil
}

func loadSegmentsAndMembers(ctx context.Context, q sqlRunner, m *mission.DiveMission) error {
	rows, err := q.QueryContext(ctx, `SELECT name FROM mission_segments WHERE mission_id=? ORDER BY position`, m.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return err
		}
		m.Segments = append(m.Segments, value)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = q.QueryContext(ctx, `SELECT person_id,name,role FROM mission_members WHERE mission_id=? ORDER BY role,person_id`, m.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value mission.Member
		if err := rows.Scan(&value.PersonID, &value.Name, &value.Role); err != nil {
			return err
		}
		m.TeamMembers = append(m.TeamMembers, value)
	}
	return rows.Err()
}

func replaceRisks(ctx context.Context, tx *sql.Tx, m *mission.DiveMission) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM segment_risks WHERE mission_id=?`, m.ID); err != nil {
		return err
	}
	for _, risk := range m.Risks {
		hazards, _ := json.Marshal(risk.Hazards)
		mitigations, _ := json.Marshal(risk.Mitigations)
		breakdown, _ := json.Marshal(risk.ScoreBreakdown)
		actions, _ := json.Marshal(risk.MitigationActions)
		_, err := tx.ExecContext(ctx, `INSERT INTO segment_risks(id,mission_id,segment_name,current_level,visibility_m,restriction_grade,exit_limit_min,hazards,mitigations,risk_level,risk_explanation,score,score_breakdown,assessed_by,mitigation_actions) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			risk.ID, m.ID, risk.SegmentName, risk.CurrentLevel, risk.VisibilityM, risk.RestrictionGrade, risk.ExitLimitMin, hazards, mitigations, risk.RiskLevel, risk.RiskExplanation, risk.Score, breakdown, risk.AssessedBy, actions)
		if err != nil {
			return err
		}
	}
	return nil
}

func loadRisks(ctx context.Context, q sqlRunner, m *mission.DiveMission) error {
	rows, err := q.QueryContext(ctx, `SELECT id,segment_name,current_level,visibility_m,restriction_grade,exit_limit_min,hazards,mitigations,risk_level,risk_explanation,score,score_breakdown,assessed_by,mitigation_actions FROM segment_risks WHERE mission_id=? ORDER BY segment_name`, m.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r mission.SegmentRisk
		var hazards, mitigations, breakdown, actions []byte
		r.MissionID = m.ID
		if err := rows.Scan(&r.ID, &r.SegmentName, &r.CurrentLevel, &r.VisibilityM, &r.RestrictionGrade, &r.ExitLimitMin, &hazards, &mitigations, &r.RiskLevel, &r.RiskExplanation, &r.Score, &breakdown, &r.AssessedBy, &actions); err != nil {
			return err
		}
		if err := json.Unmarshal(hazards, &r.Hazards); err != nil {
			return err
		}
		if err := json.Unmarshal(mitigations, &r.Mitigations); err != nil {
			return err
		}
		if err := json.Unmarshal(breakdown, &r.ScoreBreakdown); err != nil {
			return err
		}
		if len(actions) > 0 {
			_ = json.Unmarshal(actions, &r.MitigationActions)
		}
		m.Risks = append(m.Risks, r)
	}
	return rows.Err()
}

func replacePlan(ctx context.Context, tx *sql.Tx, m *mission.DiveMission) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM life_support_plans WHERE mission_id=?`, m.ID); err != nil {
		return err
	}
	if m.LifeSupportPlan == nil {
		return nil
	}
	p := m.LifeSupportPlan
	members, _ := json.Marshal(p.Members)
	gases, _ := json.Marshal(p.GasMixes)
	assignments, _ := json.Marshal(p.SupportAssignments)
	crossCheck, _ := json.Marshal(p.CrossCheck)
	failedRules, _ := json.Marshal(p.FailedRules)
	metadata, _ := json.Marshal(struct {
		Version              int64                         `json:"version"`
		DepthAdaptations     []mission.GasDepthAdaptation  `json:"depth_adaptations"`
		RevisedFromPlanID    string                        `json:"revised_from_plan_id"`
		RemediationNotes     map[string]string             `json:"remediation_notes"`
		RevisionDiff         map[string]any                `json:"revision_diff"`
		MemberGasAssignments []mission.MemberGasAssignment `json:"member_gas_assignments"`
		MemberGasMargins     []mission.MemberGasMargin     `json:"member_gas_margins"`
		SegmentGasBudgets    []mission.SegmentGasBudget    `json:"segment_gas_budgets"`
		ScenarioMargins      []mission.GasFailureScenario  `json:"scenario_margins"`
		BudgetRuleVersion    string                        `json:"budget_rule_version"`
	}{p.Version, p.DepthAdaptations, p.RevisedFromPlanID, p.RemediationNotes, p.RevisionDiff, p.MemberGasAssignments, p.MemberGasMargins, p.SegmentGasBudgets, p.ScenarioMargins, p.BudgetRuleVersion})
	_, err := tx.ExecContext(ctx, `INSERT INTO life_support_plans(id,mission_id,members,gas_mixes,turn_pressure_bar,reserve_rule,support_assignments,review_status,reviewed_by,review_note,cross_check,failed_rules,plan_metadata) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, m.ID, members, gases, p.TurnPressureBar, p.ReserveRule, assignments, p.ReviewStatus, p.ReviewedBy, p.ReviewNote, crossCheck, failedRules, metadata)
	return err
}

func loadPlan(ctx context.Context, q sqlRunner, m *mission.DiveMission) error {
	p := &mission.LifeSupportPlan{MissionID: m.ID}
	var members, gases, assignments, crossCheck, failedRules, metadata []byte
	err := q.QueryRowContext(ctx, `SELECT id,members,gas_mixes,turn_pressure_bar,reserve_rule,support_assignments,review_status,reviewed_by,review_note,cross_check,failed_rules,plan_metadata FROM life_support_plans WHERE mission_id=?`, m.ID).Scan(&p.ID, &members, &gases, &p.TurnPressureBar, &p.ReserveRule, &assignments, &p.ReviewStatus, &p.ReviewedBy, &p.ReviewNote, &crossCheck, &failedRules, &metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(members, &p.Members); err != nil {
		return err
	}
	if err = json.Unmarshal(gases, &p.GasMixes); err != nil {
		return err
	}
	if err = json.Unmarshal(assignments, &p.SupportAssignments); err != nil {
		return err
	}
	if err = json.Unmarshal(crossCheck, &p.CrossCheck); err != nil {
		return err
	}
	if err = json.Unmarshal(failedRules, &p.FailedRules); err != nil {
		return err
	}
	var meta struct {
		Version              int64                         `json:"version"`
		DepthAdaptations     []mission.GasDepthAdaptation  `json:"depth_adaptations"`
		RevisedFromPlanID    string                        `json:"revised_from_plan_id"`
		RemediationNotes     map[string]string             `json:"remediation_notes"`
		RevisionDiff         map[string]any                `json:"revision_diff"`
		MemberGasAssignments []mission.MemberGasAssignment `json:"member_gas_assignments"`
		MemberGasMargins     []mission.MemberGasMargin     `json:"member_gas_margins"`
		SegmentGasBudgets    []mission.SegmentGasBudget    `json:"segment_gas_budgets"`
		ScenarioMargins      []mission.GasFailureScenario  `json:"scenario_margins"`
		BudgetRuleVersion    string                        `json:"budget_rule_version"`
	}
	if len(metadata) > 0 && json.Unmarshal(metadata, &meta) == nil {
		p.Version = meta.Version
		p.DepthAdaptations = meta.DepthAdaptations
		p.RevisedFromPlanID = meta.RevisedFromPlanID
		p.RemediationNotes = meta.RemediationNotes
		p.RevisionDiff = meta.RevisionDiff
		p.MemberGasAssignments = meta.MemberGasAssignments
		p.MemberGasMargins = meta.MemberGasMargins
		p.SegmentGasBudgets, p.ScenarioMargins, p.BudgetRuleVersion = meta.SegmentGasBudgets, meta.ScenarioMargins, meta.BudgetRuleVersion
	}
	if p.Version == 0 {
		p.Version = int64(len(m.PlanHistory) + 1)
	}
	m.LifeSupportPlan = p
	return nil
}

func replaceVerifications(ctx context.Context, tx *sql.Tx, m *mission.DiveMission) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM verification_records WHERE mission_id=?`, m.ID); err != nil {
		return err
	}
	for _, r := range m.Verifications {
		metadata, _ := json.Marshal(struct {
			FailureReason              string                         `json:"failure_reason"`
			ReplacementForAssetID      string                         `json:"replacement_for_asset_id"`
			ReplacementReason          string                         `json:"replacement_reason"`
			RuleVersion                string                         `json:"rule_version"`
			RequiredMaxDurationSeconds int                            `json:"required_max_duration_seconds"`
			CompletedSteps             []string                       `json:"completed_steps"`
			DeviationCodes             []string                       `json:"deviation_codes"`
			RemediationCycle           int                            `json:"remediation_cycle"`
			ReferencedRecordID         string                         `json:"referenced_record_id"`
			Measurements               map[string]mission.Measurement `json:"measurements"`
			MeasurementResults         []mission.MeasurementResult    `json:"measurement_results"`
			RemediationDueAt           time.Time                      `json:"remediation_due_at"`
			DelayReason                string                         `json:"delay_reason"`
			DelayReviewedBy            string                         `json:"delay_reviewed_by"`
			WasOverdue                 bool                           `json:"was_overdue"`
			DelaySeconds               int64                          `json:"delay_seconds"`
		}{r.FailureReason, r.ReplacementForAssetID, r.ReplacementReason, r.RuleVersion, r.RequiredMaxDurationSeconds, r.CompletedSteps, r.DeviationCodes, r.RemediationCycle, r.ReferencedRecordID, r.Measurements, r.MeasurementResults, r.RemediationDueAt, r.DelayReason, r.DelayReviewedBy, r.WasOverdue, r.DelaySeconds})
		_, err := tx.ExecContext(ctx, `INSERT INTO verification_records(id,mission_id,record_type,check_code,outcome,evidence_digest,deviation,corrective_action,review_marker,verified_by,recorded_at,asset_id,inspected_at,valid_until,conducted_at,duration_seconds,verification_metadata) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, m.ID, r.RecordType, r.CheckCode, r.Outcome, r.EvidenceDigest, r.Deviation, r.CorrectiveAction, r.ReviewMarker, r.VerifiedBy, stamp(r.RecordedAt), r.AssetID, optionalStamp(r.InspectedAt), optionalStamp(r.ValidUntil), optionalStamp(r.ConductedAt), r.DurationSeconds, metadata)
		if err != nil {
			return err
		}
	}
	return nil
}

func loadVerifications(ctx context.Context, q sqlRunner, m *mission.DiveMission) error {
	rows, err := q.QueryContext(ctx, `SELECT id,record_type,check_code,outcome,evidence_digest,deviation,corrective_action,review_marker,verified_by,recorded_at,asset_id,inspected_at,valid_until,conducted_at,duration_seconds,verification_metadata FROM verification_records WHERE mission_id=? ORDER BY recorded_at,record_type,check_code`, m.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r mission.VerificationRecord
		var recorded, inspected, validUntil, conducted string
		var metadata []byte
		r.MissionID = m.ID
		if err := rows.Scan(&r.ID, &r.RecordType, &r.CheckCode, &r.Outcome, &r.EvidenceDigest, &r.Deviation, &r.CorrectiveAction, &r.ReviewMarker, &r.VerifiedBy, &recorded, &r.AssetID, &inspected, &validUntil, &conducted, &r.DurationSeconds, &metadata); err != nil {
			return err
		}
		r.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded)
		if err != nil {
			return err
		}
		if inspected != "" {
			r.InspectedAt, err = time.Parse(time.RFC3339Nano, inspected)
			if err != nil {
				return err
			}
		}
		if validUntil != "" {
			r.ValidUntil, err = time.Parse(time.RFC3339Nano, validUntil)
			if err != nil {
				return err
			}
		}
		if conducted != "" {
			r.ConductedAt, err = time.Parse(time.RFC3339Nano, conducted)
			if err != nil {
				return err
			}
		}
		var meta struct {
			FailureReason              string                         `json:"failure_reason"`
			ReplacementForAssetID      string                         `json:"replacement_for_asset_id"`
			ReplacementReason          string                         `json:"replacement_reason"`
			RuleVersion                string                         `json:"rule_version"`
			RequiredMaxDurationSeconds int                            `json:"required_max_duration_seconds"`
			CompletedSteps             []string                       `json:"completed_steps"`
			DeviationCodes             []string                       `json:"deviation_codes"`
			RemediationCycle           int                            `json:"remediation_cycle"`
			ReferencedRecordID         string                         `json:"referenced_record_id"`
			Measurements               map[string]mission.Measurement `json:"measurements"`
			MeasurementResults         []mission.MeasurementResult    `json:"measurement_results"`
			RemediationDueAt           time.Time                      `json:"remediation_due_at"`
			DelayReason                string                         `json:"delay_reason"`
			DelayReviewedBy            string                         `json:"delay_reviewed_by"`
			WasOverdue                 bool                           `json:"was_overdue"`
			DelaySeconds               int64                          `json:"delay_seconds"`
		}
		if len(metadata) > 0 && json.Unmarshal(metadata, &meta) == nil {
			r.FailureReason, r.ReplacementForAssetID, r.ReplacementReason = meta.FailureReason, meta.ReplacementForAssetID, meta.ReplacementReason
			r.RuleVersion, r.RequiredMaxDurationSeconds, r.CompletedSteps, r.DeviationCodes = meta.RuleVersion, meta.RequiredMaxDurationSeconds, meta.CompletedSteps, meta.DeviationCodes
			r.RemediationCycle, r.ReferencedRecordID = meta.RemediationCycle, meta.ReferencedRecordID
			r.Measurements, r.MeasurementResults, r.RemediationDueAt = meta.Measurements, meta.MeasurementResults, meta.RemediationDueAt
			r.DelayReason, r.DelayReviewedBy, r.WasOverdue, r.DelaySeconds = meta.DelayReason, meta.DelayReviewedBy, meta.WasOverdue, meta.DelaySeconds
		}
		m.Verifications = append(m.Verifications, r)
	}
	return rows.Err()
}

func optionalStamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return stamp(t)
}
