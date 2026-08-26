package mission

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusDraft                 Status = "draft"
	StatusRiskAssessed          Status = "risk_assessed"
	StatusPlanReview            Status = "plan_review"
	StatusEquipmentVerification Status = "equipment_verification"
	StatusDrillPending          Status = "drill_pending"
	StatusRemediation           Status = "remediation"
	StatusReadyForRelease       Status = "ready_for_release"
	StatusReleaseRejected       Status = "release_rejected"
	StatusSigned                Status = "signed"
	StatusArchived              Status = "archived"
)

type Member struct {
	PersonID string `json:"person_id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

type MemberQualification struct {
	PersonID          string    `json:"person_id"`
	QualificationCode string    `json:"qualification_code"`
	Level             int       `json:"qualification_level"`
	LegacyLevel       int       `json:"level,omitempty"`
	ValidUntil        time.Time `json:"valid_until"`
	EvidenceDigest    string    `json:"evidence_digest"`
}

type QualificationStatus struct {
	PersonID              string   `json:"person_id"`
	Role                  string   `json:"role"`
	Passed                bool     `json:"passed"`
	RequiredCode          string   `json:"required_code"`
	RequiredLevel         int      `json:"required_level"`
	MissingQualifications []string `json:"missing_qualifications,omitempty"`
	EvidenceDigest        string   `json:"evidence_digest,omitempty"`
	RemainingValidDays    int64    `json:"remaining_valid_days"`
}

type DiveMission struct {
	ID                     string                   `json:"id"`
	Title                  string                   `json:"title"`
	CaveSite               string                   `json:"cave_site"`
	CaveSiteKey            string                   `json:"cave_site_key"`
	TargetDepthM           float64                  `json:"target_depth_m"`
	WindowStart            time.Time                `json:"window_start"`
	WindowEnd              time.Time                `json:"window_end"`
	Status                 Status                   `json:"status"`
	Revision               int64                    `json:"revision"`
	LeaderID               string                   `json:"leader_id"`
	Segments               []string                 `json:"segments"`
	TeamMembers            []Member                 `json:"team_members"`
	MemberQualifications   []MemberQualification    `json:"member_qualifications"`
	QualificationStatus    []QualificationStatus    `json:"qualification_status"`
	Risks                  []SegmentRisk            `json:"risks"`
	RiskHistory            []SegmentRisk            `json:"risk_history,omitempty"`
	ScheduleCheck          ScheduleCheck            `json:"schedule_check"`
	RiskSummary            RiskSummary              `json:"risk_summary"`
	LifeSupportPlan        *LifeSupportPlan         `json:"life_support_plan,omitempty"`
	Verifications          []VerificationRecord     `json:"verifications"`
	ReleaseDigest          string                   `json:"release_digest,omitempty"`
	ReleaseChecklist       []ChecklistItem          `json:"release_checklist,omitempty"`
	SignedBy               string                   `json:"signed_by,omitempty"`
	ArchiveDigest          string                   `json:"archive_digest,omitempty"`
	TemplateMissionID      string                   `json:"template_mission_id,omitempty"`
	TemplateArchiveDigest  string                   `json:"template_archive_digest,omitempty"`
	LastReleaseRejection   *ReleaseRejection        `json:"last_release_rejection,omitempty"`
	PlanHistory            []LifeSupportPlanVersion `json:"life_support_plan_history,omitempty"`
	VerificationHistory    []VerificationRecord     `json:"verification_history,omitempty"`
	CreatedAt              time.Time                `json:"created_at"`
	UpdatedAt              time.Time                `json:"updated_at"`
	ArchivedAt             *time.Time               `json:"archived_at,omitempty"`
	CycleStatuses          []map[string]any         `json:"cycle_statuses,omitempty"`
	RemediationDeadlines   []RemediationDeadline    `json:"remediation_deadlines,omitempty"`
	RiskMitigationBlockers []map[string]any         `json:"risk_mitigation_blockers,omitempty"`
}

type SegmentRisk struct {
	ID                string             `json:"id"`
	MissionID         string             `json:"mission_id"`
	SegmentName       string             `json:"segment_name"`
	CurrentLevel      int                `json:"current_level"`
	VisibilityM       float64            `json:"visibility_m"`
	RestrictionGrade  int                `json:"restriction_grade"`
	ExitLimitMin      int                `json:"exit_limit_min"`
	Hazards           []string           `json:"hazards"`
	Mitigations       []string           `json:"mitigations"`
	RiskLevel         string             `json:"risk_level"`
	RiskExplanation   string             `json:"risk_explanation"`
	Score             int                `json:"score"`
	ScoreBreakdown    RiskScoreBreakdown `json:"score_breakdown"`
	AssessedBy        string             `json:"assessed_by"`
	MitigationActions []MitigationAction `json:"mitigation_actions,omitempty"`
}

type MitigationAction struct {
	Code               string     `json:"action_code"`
	LegacyCode         string     `json:"code,omitempty"`
	Hazard             string     `json:"hazard"`
	OwnerPersonID      string     `json:"owner_person_id"`
	DueAt              time.Time  `json:"due_at"`
	CompletionCriteria string     `json:"completion_criteria"`
	Status             string     `json:"status"`
	Version            int        `json:"version"`
	PreviousActionCode string     `json:"previous_action_code,omitempty"`
	Result             string     `json:"result,omitempty"`
	EvidenceDigest     string     `json:"evidence_digest,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

type RiskScoreBreakdown struct {
	CurrentLevel     int `json:"current_level"`
	RestrictionGrade int `json:"restriction_grade"`
	Visibility       int `json:"visibility"`
	ExitLimit        int `json:"exit_limit"`
}

type RiskSummary struct {
	HighestLevel string         `json:"highest_level"`
	TotalScore   int            `json:"total_score"`
	Distribution map[string]int `json:"distribution"`
}

type ScheduleConflict struct {
	MissionID   string    `json:"mission_id"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	Status      Status    `json:"status"`
}

type ScheduleCheck struct {
	Checked     bool               `json:"checked"`
	CaveSiteKey string             `json:"cave_site_key"`
	Conflicts   []ScheduleConflict `json:"conflicts"`
}

type GasMix struct {
	Name             string  `json:"name"`
	OxygenPercent    float64 `json:"oxygen_percent"`
	HeliumPercent    float64 `json:"helium_percent"`
	StartPressureBar int     `json:"start_pressure_bar"`
	CylinderLiters   float64 `json:"cylinder_liters"`
	IntendedDepthM   float64 `json:"intended_depth_m"`
}

type SupportAssignment struct {
	PersonID string `json:"person_id"`
	Duty     string `json:"duty"`
}

type LifeSupportPlan struct {
	ID                   string                `json:"id"`
	Version              int64                 `json:"version"`
	MissionID            string                `json:"mission_id"`
	Members              []Member              `json:"members"`
	GasMixes             []GasMix              `json:"gas_mixes"`
	TurnPressureBar      int                   `json:"turn_pressure_bar"`
	ReserveRule          string                `json:"reserve_rule"`
	SupportAssignments   []SupportAssignment   `json:"support_assignments"`
	ReviewStatus         string                `json:"review_status"`
	ReviewedBy           string                `json:"reviewed_by,omitempty"`
	ReviewNote           string                `json:"review_note,omitempty"`
	CrossCheck           GasCrossCheck         `json:"cross_check"`
	FailedRules          []string              `json:"failed_rules,omitempty"`
	DepthAdaptations     []GasDepthAdaptation  `json:"depth_adaptations,omitempty"`
	RevisedFromPlanID    string                `json:"revised_from_plan_id,omitempty"`
	RemediationNotes     map[string]string     `json:"remediation_notes,omitempty"`
	RevisionDiff         map[string]any        `json:"revision_diff,omitempty"`
	MemberGasAssignments []MemberGasAssignment `json:"member_gas_assignments,omitempty"`
	MemberGasMargins     []MemberGasMargin     `json:"member_gas_margins,omitempty"`
	SegmentGasBudgets    []SegmentGasBudget    `json:"segment_gas_budgets,omitempty"`
	ScenarioMargins      []GasFailureScenario  `json:"scenario_margins,omitempty"`
	BudgetRuleVersion    string                `json:"budget_rule_version,omitempty"`
}

type SegmentGasBudget struct {
	SegmentName            string              `json:"segment_name"`
	ExpectedDepthM         float64             `json:"expected_depth_m"`
	OneWayMinutes          float64             `json:"one_way_minutes"`
	ConsumptionMultipliers map[string]float64  `json:"consumption_multipliers"`
	AvailableSources       map[string][]string `json:"available_sources"`
}

type GasFailureScenario struct {
	PersonID        string  `json:"person_id"`
	SegmentName     string  `json:"segment_name"`
	FailedSource    string  `json:"failed_source"`
	RequiredLiters  float64 `json:"required_liters"`
	AvailableLiters float64 `json:"available_liters"`
	MarginLiters    float64 `json:"margin_liters"`
	Passed          bool    `json:"passed"`
}

type MemberGasAssignment struct {
	PersonID               string    `json:"person_id"`
	SurfaceConsumptionLMin float64   `json:"surface_consumption_l_min"`
	PrimaryAssetID         string    `json:"primary_asset_id"`
	PrimaryGasMix          string    `json:"primary_gas_mix"`
	RedundantAssetID       string    `json:"redundant_asset_id"`
	RedundantGasMix        string    `json:"redundant_gas_mix"`
	Primary                GasSource `json:"primary,omitempty"`
	Redundant              GasSource `json:"redundant,omitempty"`
}

type GasSource struct {
	AssetID string `json:"asset_id"`
	GasMix  string `json:"gas_mix"`
}

type MemberGasMargin struct {
	PersonID        string  `json:"person_id"`
	SourceRole      string  `json:"source_role"`
	AssetID         string  `json:"asset_id"`
	GasMix          string  `json:"gas_mix"`
	AvailableLiters float64 `json:"available_liters"`
	RequiredLiters  float64 `json:"required_liters"`
	MarginLiters    float64 `json:"margin_liters"`
	Passed          bool    `json:"passed"`
}

type LifeSupportPlanVersion struct {
	Plan       LifeSupportPlan `json:"plan"`
	Version    int64           `json:"version"`
	RejectedBy string          `json:"rejected_by,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	Revision   int64           `json:"revision"`
}

type GasDepthAdaptation struct {
	GasName                  string  `json:"gas_name"`
	IntendedDepthM           float64 `json:"intended_depth_m"`
	OxygenPartialPressureBar float64 `json:"oxygen_partial_pressure_bar"`
	MaximumOperatingDepthM   float64 `json:"maximum_operating_depth_m"`
	EquivalentNarcoticDepthM float64 `json:"equivalent_narcotic_depth_m"`
	Passed                   bool    `json:"passed"`
}

type GasMargin struct {
	GasName            string `json:"gas_name"`
	Role               string `json:"role"`
	AvailableBar       int    `json:"available_bar"`
	RequiredReserveBar int    `json:"required_reserve_bar"`
	MarginBar          int    `json:"margin_bar"`
	Passed             bool   `json:"passed"`
}

type GasCrossCheck struct {
	Passed         bool        `json:"passed"`
	TargetDepthBar int         `json:"target_depth_bar"`
	Margins        []GasMargin `json:"margins"`
}

type VerificationRecord struct {
	ID                         string                 `json:"id"`
	MissionID                  string                 `json:"mission_id"`
	RecordType                 string                 `json:"record_type"`
	CheckCode                  string                 `json:"check_code"`
	Outcome                    string                 `json:"outcome"`
	EvidenceDigest             string                 `json:"evidence_digest"`
	Deviation                  string                 `json:"deviation,omitempty"`
	CorrectiveAction           string                 `json:"corrective_action,omitempty"`
	ReviewMarker               string                 `json:"review_marker,omitempty"`
	VerifiedBy                 string                 `json:"verified_by"`
	RecordedAt                 time.Time              `json:"recorded_at"`
	AssetID                    string                 `json:"asset_id,omitempty"`
	InspectedAt                time.Time              `json:"inspected_at,omitempty"`
	ValidUntil                 time.Time              `json:"valid_until,omitempty"`
	ConductedAt                time.Time              `json:"conducted_at,omitempty"`
	DurationSeconds            int                    `json:"duration_seconds,omitempty"`
	FailureReason              string                 `json:"failure_reason,omitempty"`
	ReplacementForAssetID      string                 `json:"replacement_for_asset_id,omitempty"`
	ReplacementReason          string                 `json:"replacement_reason,omitempty"`
	RuleVersion                string                 `json:"rule_version,omitempty"`
	RequiredMaxDurationSeconds int                    `json:"required_max_duration_seconds,omitempty"`
	CompletedSteps             []string               `json:"completed_steps,omitempty"`
	DeviationCodes             []string               `json:"deviation_codes,omitempty"`
	RemediationCycle           int                    `json:"remediation_cycle,omitempty"`
	ReferencedRecordID         string                 `json:"referenced_record_id,omitempty"`
	Measurements               map[string]Measurement `json:"measurements,omitempty"`
	MeasurementResults         []MeasurementResult    `json:"measurement_results,omitempty"`
	RemediationDueAt           time.Time              `json:"remediation_due_at,omitempty"`
	DelayReason                string                 `json:"delay_reason,omitempty"`
	DelayReviewedBy            string                 `json:"delay_reviewed_by,omitempty"`
	WasOverdue                 bool                   `json:"was_overdue,omitempty"`
	DelaySeconds               int64                  `json:"delay_seconds,omitempty"`
}

type Measurement struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type MeasurementResult struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Threshold float64 `json:"threshold"`
	Operator  string  `json:"operator"`
	Margin    float64 `json:"margin"`
	Passed    bool    `json:"passed"`
}

type RemediationDeadline struct {
	CheckCode        string    `json:"check_code"`
	DueAt            time.Time `json:"due_at"`
	Status           string    `json:"status"`
	RemainingSeconds int64     `json:"remaining_seconds"`
	Overdue          bool      `json:"overdue"`
	DelaySeconds     int64     `json:"delay_seconds,omitempty"`
	DelayReason      string    `json:"delay_reason,omitempty"`
	DelayReviewedBy  string    `json:"delay_reviewed_by,omitempty"`
	RuleVersion      string    `json:"rule_version"`
}

type ChecklistItem struct {
	Code           string              `json:"code"`
	Label          string              `json:"label"`
	Passed         bool                `json:"passed"`
	Detail         string              `json:"detail"`
	CurrentValue   string              `json:"current_value"`
	RequiredValue  string              `json:"required_value"`
	MissingReason  string              `json:"missing_reason,omitempty"`
	SourceRecords  []EvidenceReference `json:"source_records,omitempty"`
	Lineage        []EvidenceReference `json:"lineage,omitempty"`
	RequiredAction string              `json:"required_action,omitempty"`
	Endpoint       string              `json:"endpoint,omitempty"`
}

type EvidenceReference struct {
	GateCode        string    `json:"gate_code"`
	RecordCode      string    `json:"record_code"`
	RecordID        string    `json:"record_id"`
	BusinessVersion int64     `json:"business_version"`
	ActorID         string    `json:"actor_id"`
	OccurredAt      time.Time `json:"occurred_at"`
	AuditSequence   int64     `json:"audit_sequence"`
	Relationship    string    `json:"relationship,omitempty"`
}

type ReleaseRejection struct {
	Reason        string          `json:"reason"`
	ActorID       string          `json:"actor_id"`
	Revision      int64           `json:"revision"`
	Checklist     []ChecklistItem `json:"checklist"`
	PreviewDigest string          `json:"preview_digest"`
	Cycle         int             `json:"cycle"`
}

type HistoryEntry struct {
	Status     Status    `json:"status"`
	Revision   int64     `json:"revision"`
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
}

type CommandMeta struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	ActorID          string `json:"actor_id"`
}

type CommandResult struct {
	Mission          *DiveMission `json:"mission"`
	AllowedActions   []string     `json:"allowed_actions"`
	IdempotentReplay bool         `json:"idempotent_replay,omitempty"`
}

type DraftRevisionInput struct {
	Meta                 CommandMeta            `json:"-"`
	Title                *string                `json:"title,omitempty"`
	CaveSite             *string                `json:"cave_site,omitempty"`
	TargetDepthM         *float64               `json:"target_depth_m,omitempty"`
	WindowStart          *time.Time             `json:"window_start,omitempty"`
	WindowEnd            *time.Time             `json:"window_end,omitempty"`
	Segments             *[]string              `json:"segments,omitempty"`
	TeamMembers          *[]Member              `json:"team_members,omitempty"`
	MemberQualifications *[]MemberQualification `json:"member_qualifications,omitempty"`
}

type RiskReassessmentInput struct {
	Meta   CommandMeta   `json:"-"`
	Reason string        `json:"reason"`
	Risks  []SegmentRisk `json:"risks"`
}

type DrillBatchInput struct {
	Meta  CommandMeta  `json:"-"`
	Items []DrillInput `json:"items"`
}

type StoredResult struct {
	StatusCode int             `json:"status_code"`
	Body       json.RawMessage `json:"body"`
}
