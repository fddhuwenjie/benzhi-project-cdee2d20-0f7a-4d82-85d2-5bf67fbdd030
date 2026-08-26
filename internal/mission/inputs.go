package mission

import "time"

type CreateInput struct {
	Meta                 CommandMeta           `json:"-"`
	Title                string                `json:"title"`
	CaveSite             string                `json:"cave_site"`
	TargetDepthM         float64               `json:"target_depth_m"`
	WindowStart          time.Time             `json:"window_start"`
	WindowEnd            time.Time             `json:"window_end"`
	Segments             []string              `json:"segments"`
	TeamMembers          []Member              `json:"team_members"`
	MemberQualifications []MemberQualification `json:"member_qualifications"`
	SourceDigest         string                `json:"source_digest,omitempty"`
	TemplateMissionID    string                `json:"template_mission_id,omitempty"`
}

type RiskInput struct {
	Meta              CommandMeta   `json:"-"`
	Risks             []SegmentRisk `json:"risks"`
	ValidateOnly      bool          `json:"validate_only,omitempty"`
	RiskPreviewDigest string        `json:"risk_preview_digest,omitempty"`
}

type PlanInput struct {
	Meta                 CommandMeta           `json:"-"`
	Members              []Member              `json:"members"`
	GasMixes             []GasMix              `json:"gas_mixes"`
	TurnPressureBar      int                   `json:"turn_pressure_bar"`
	ReserveRule          string                `json:"reserve_rule"`
	SupportAssignments   []SupportAssignment   `json:"support_assignments"`
	RevisedFromPlanID    string                `json:"revised_from_plan_id,omitempty"`
	RemediationNotes     map[string]string     `json:"remediation_notes,omitempty"`
	MemberGasAssignments []MemberGasAssignment `json:"member_gas_assignments,omitempty"`
	SegmentGasBudgets    []SegmentGasBudget    `json:"segment_gas_budgets,omitempty"`
}

type ReviewInput struct {
	Meta        CommandMeta `json:"-"`
	Decision    string      `json:"decision"`
	Reason      string      `json:"reason"`
	FailedRules []string    `json:"failed_rules,omitempty"`
}

type EquipmentInput struct {
	Meta                  CommandMeta            `json:"-"`
	CheckCode             string                 `json:"check_code"`
	Outcome               string                 `json:"outcome"`
	EvidenceDigest        string                 `json:"evidence_digest"`
	ReviewMarker          string                 `json:"review_marker,omitempty"`
	AssetID               string                 `json:"asset_id"`
	InspectedAt           time.Time              `json:"inspected_at"`
	ValidUntil            time.Time              `json:"valid_until"`
	FailureReason         string                 `json:"failure_reason,omitempty"`
	ReplacementForAssetID string                 `json:"replacement_for_asset_id,omitempty"`
	ReplacementReason     string                 `json:"replacement_reason,omitempty"`
	Measurements          map[string]Measurement `json:"measurements"`
}

type EquipmentBatchInput struct {
	Meta  CommandMeta      `json:"-"`
	Items []EquipmentInput `json:"items"`
}

type DrillInput struct {
	Meta                    CommandMeta `json:"-"`
	CheckCode               string      `json:"check_code"`
	Outcome                 string      `json:"outcome"`
	EvidenceDigest          string      `json:"evidence_digest"`
	Deviation               string      `json:"deviation"`
	ConductedAt             time.Time   `json:"conducted_at"`
	DurationSeconds         int         `json:"duration_seconds"`
	ObservedDurationSeconds int         `json:"observed_duration_seconds,omitempty"`
	CompletedSteps          []string    `json:"completed_steps,omitempty"`
}

type RemediationInput struct {
	Meta               CommandMeta `json:"-"`
	CheckCode          string      `json:"check_code"`
	CorrectiveAction   string      `json:"corrective_action"`
	EvidenceDigest     string      `json:"evidence_digest"`
	CompletedAt        time.Time   `json:"completed_at,omitempty"`
	Cycle              int         `json:"cycle,omitempty"`
	ReferencedRecordID string      `json:"referenced_record_id,omitempty"`
	DelayReason        string      `json:"delay_reason,omitempty"`
	DelayReviewedBy    string      `json:"delay_reviewed_by,omitempty"`
}

type MitigationCompletionInput struct {
	Meta           CommandMeta `json:"-"`
	ActionCode     string      `json:"action_code"`
	Result         string      `json:"result"`
	EvidenceDigest string      `json:"evidence_digest"`
	CompletedAt    time.Time   `json:"completed_at,omitempty"`
}

type MitigationBatchItem struct {
	ActionCode     string    `json:"action_code"`
	Result         string    `json:"result"`
	EvidenceDigest string    `json:"evidence_digest"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
}

type MitigationBatchInput struct {
	Meta  CommandMeta           `json:"-"`
	Items []MitigationBatchItem `json:"items"`
}

type RemediationBatchInput struct {
	Meta  CommandMeta        `json:"-"`
	Items []RemediationInput `json:"items"`
}

type RetestInput struct {
	Meta               CommandMeta `json:"-"`
	CheckCode          string      `json:"check_code"`
	Outcome            string      `json:"outcome"`
	EvidenceDigest     string      `json:"evidence_digest"`
	Cycle              int         `json:"cycle,omitempty"`
	ReferencedRecordID string      `json:"referenced_record_id,omitempty"`
}

type RetestBatchInput struct {
	Meta  CommandMeta   `json:"-"`
	Items []RetestInput `json:"items"`
}

type ReleaseInput struct {
	Meta              CommandMeta `json:"-"`
	Decision          string      `json:"decision"`
	Reason            string      `json:"reason"`
	PreviewDigest     string      `json:"preview_digest,omitempty"`
	SourceRevision    int64       `json:"source_revision,omitempty"`
	RejectionRevision int64       `json:"rejection_revision,omitempty"`
	AcknowledgeReason string      `json:"acknowledge_reason,omitempty"`
}

type ArchiveInput struct {
	Meta CommandMeta `json:"-"`
}
