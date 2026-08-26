package mission

import (
	"context"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

type Tx interface {
	LoadMission(context.Context, string) (*DiveMission, error)
	InsertMission(context.Context, *DiveMission) error
	SaveMission(context.Context, *DiveMission, int64) error
	AppendEvent(context.Context, audit.Event) (audit.Event, error)
	AllEvents(context.Context, string) ([]audit.Event, error)
	ScheduleConflicts(context.Context, string, time.Time, time.Time) ([]ScheduleConflict, error)
	ScheduleConflictsExcluding(context.Context, string, time.Time, time.Time, string) ([]ScheduleConflict, error)
}

type Repository interface {
	Execute(context.Context, string, string, string, string, func(Tx) (StoredResult, error)) (StoredResult, bool, error)
	Mission(context.Context, string) (*DiveMission, error)
	History(context.Context, string) ([]HistoryEntry, error)
	List(context.Context, ListFilter) (ListResult, error)
	SchedulePreflight(context.Context, string, time.Time, time.Time) ([]ScheduleConflict, error)
	AllEvents(context.Context, string) ([]audit.Event, error)
}

type ArchiveLister interface {
	ListArchived(context.Context, ArchiveFilter) ([]ArchiveCandidate, error)
}

// UnverifiedMissionReader 供只读完整性巡检读取原始档案，避免单个损坏档案阻断整批结果。
type UnverifiedMissionReader interface {
	MissionUnverified(context.Context, string) (*DiveMission, error)
}

type ArchiveFilter struct {
	CaveSite string
	From, To *time.Time
}

type ArchiveCandidate struct {
	ID         string
	CaveSite   string
	ArchivedAt time.Time
}

type ListFilter struct {
	Status               string
	CaveSite             string
	LeaderID             string
	WindowFrom, WindowTo *time.Time
	Limit                int
	Cursor               string
	RiskLevel            string
	MinTotalScore        int
	MitigationState      string
}
type MissionSummary struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	CaveSite          string    `json:"cave_site"`
	Status            Status    `json:"status"`
	Revision          int64     `json:"revision"`
	LeaderID          string    `json:"leader_id"`
	WindowStart       time.Time `json:"window_start"`
	WindowEnd         time.Time `json:"window_end"`
	AllowedActions    []string  `json:"allowed_actions"`
	HighestRisk       string    `json:"highest_risk"`
	EquipmentComplete bool      `json:"equipment_complete"`
	TotalRiskScore    int       `json:"total_risk_score"`
	MitigationState   string    `json:"mitigation_state"`
}
type ListStatistics struct {
	StatusCounts             map[string]int `json:"status_counts"`
	RiskHighest              map[string]int `json:"risk_highest"`
	RemediationCount         int            `json:"remediation_count"`
	EquipmentIncompleteCount int            `json:"equipment_incomplete_count"`
	TotalScoreRanges         map[string]int `json:"total_score_ranges"`
	MitigationStates         map[string]int `json:"mitigation_states"`
}

func RiskMitigationState(m *DiveMission) string {
	if len(m.Risks) == 0 {
		return "none"
	}
	for _, r := range m.Risks {
		if len(r.MitigationActions) > 0 {
			for _, action := range r.MitigationActions {
				if action.Status != "completed" {
					return "incomplete"
				}
			}
			continue
		}
		required := 0
		if r.RiskLevel == "critical" {
			required = 2
		} else if r.RiskLevel == "high" {
			required = 1
		}
		if len(r.Mitigations) < required {
			return "incomplete"
		}
	}
	return "complete"
}

type ListResult struct {
	Items      []MissionSummary `json:"items"`
	Statistics ListStatistics   `json:"statistics"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type SchedulePreflight struct {
	RequestID    string             `json:"request_id"`
	ReadOnly     bool               `json:"read_only"`
	Available    bool               `json:"available"`
	CaveSiteKey  string             `json:"cave_site_key"`
	WindowStart  time.Time          `json:"window_start"`
	WindowEnd    time.Time          `json:"window_end"`
	Conflicts    []ScheduleConflict `json:"conflicts"`
	SourceDigest string             `json:"source_digest"`
}
