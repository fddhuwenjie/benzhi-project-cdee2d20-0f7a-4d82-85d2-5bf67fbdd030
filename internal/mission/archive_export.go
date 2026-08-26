package mission

import (
	"context"
	"sort"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

type ArchiveExport struct {
	Archive            *DiveMission      `json:"archive"`
	StatusHistory      []HistoryEntry    `json:"status_history"`
	Events             []audit.Event     `json:"events"`
	SegmentStartDigest string            `json:"segment_start_digest"`
	SegmentEndDigest   string            `json:"segment_end_digest"`
	ChainDigest        string            `json:"chain_digest"`
	ExportDigest       string            `json:"export_digest"`
	NextCursor         int64             `json:"next_cursor,omitempty"`
	ComplianceSummary  ComplianceSummary `json:"compliance_summary"`
	SummaryDigest      string            `json:"summary_digest"`
}

type StageTiming struct {
	Status          Status    `json:"status"`
	FirstEnteredAt  time.Time `json:"first_entered_at"`
	DurationSeconds int64     `json:"duration_seconds"`
}
type RoleIsolationCheck struct {
	CheckCode      string   `json:"check_code"`
	Status         string   `json:"status"`
	ActorIDs       []string `json:"actor_ids"`
	EventSequences []int64  `json:"event_sequences"`
}
type ComplianceSummary struct {
	StageTimings                []StageTiming        `json:"stage_timings"`
	TotalDurationSeconds        int64                `json:"total_duration_seconds"`
	PlanRejectionCount          int                  `json:"plan_rejection_count"`
	EquipmentReplacementCount   int                  `json:"equipment_replacement_count"`
	DrillDeviationCount         int                  `json:"drill_deviation_count"`
	RetestFailureCount          int                  `json:"retest_failure_count"`
	RemediationCycleCount       int                  `json:"remediation_cycle_count"`
	RoleIsolation               []RoleIsolationCheck `json:"role_isolation"`
	HighestRisk                 string               `json:"highest_risk"`
	MinimumGasMarginLiters      float64              `json:"minimum_gas_margin_liters"`
	EquipmentReplacementLineage []EvidenceReference  `json:"equipment_replacement_lineage"`
	DrillOverruns               []map[string]any     `json:"drill_overruns"`
	FinalResolutionCycles       []map[string]any     `json:"final_resolution_cycles"`
	OverdueRemediations         []map[string]any     `json:"overdue_remediations"`
}

func (s *Service) ExportArchive(ctx context.Context, id string, after int64, limit int, eventType string) (ArchiveExport, error) {
	m, err := s.repo.Mission(ctx, id)
	if err != nil {
		return ArchiveExport{}, err
	}
	if m.Status != StatusArchived {
		return ArchiveExport{}, InvalidState(m.Status, "导出归档档案")
	}
	events, err := s.repo.AllEvents(ctx, id)
	if err != nil {
		return ArchiveExport{}, NewError("archive_integrity_failed", "归档审计链读取失败", 409)
	}
	if err := audit.Verify(events); err != nil {
		return ArchiveExport{}, NewError("archive_integrity_failed", "归档审计链校验失败", 409)
	}
	if len(events) < 2 || events[len(events)-1].EventType != "mission_archived" || events[len(events)-2].EventHash != m.ArchiveDigest {
		return ArchiveExport{}, NewError("archive_integrity_failed", "归档摘要与审计链不一致", 409)
	}
	if after < 0 || len(events) > 0 && after > events[len(events)-1].Sequence {
		return ArchiveExport{}, Invalid("after", "游标超出审计链范围")
	}
	if limit <= 0 || limit > 200 {
		return ArchiveExport{}, Invalid("limit", "必须在 1 到 200 之间")
	}
	if eventType != "" {
		known := false
		for _, e := range events {
			if e.EventType == eventType {
				known = true
				break
			}
		}
		if !known {
			return ArchiveExport{}, Invalid("event_type", "未知审计事件类型")
		}
	}
	history := make([]HistoryEntry, 0, len(events))
	for _, e := range events {
		history = append(history, HistoryEntry{Status: Status(e.StatusAfter), Revision: e.ToRevision, EventType: e.EventType, OccurredAt: e.OccurredAt})
	}
	chainDigest := events[len(events)-1].EventHash
	compliance, err := complianceSummary(m, events)
	if err != nil {
		return ArchiveExport{}, NewError("archive_integrity_failed", "归档复盘证据映射失败", 409)
	}
	summaryDigest, err := audit.Digest(struct {
		Archive     *DiveMission      `json:"archive"`
		ChainDigest string            `json:"chain_digest"`
		Compliance  ComplianceSummary `json:"compliance_summary"`
	}{m, chainDigest, compliance})
	if err != nil {
		return ArchiveExport{}, err
	}
	exportDigest, err := audit.Digest(struct {
		Archive     *DiveMission  `json:"archive"`
		Events      []audit.Event `json:"events"`
		ChainDigest string        `json:"chain_digest"`
	}{m, events, chainDigest})
	if err != nil {
		return ArchiveExport{}, err
	}
	filtered := make([]audit.Event, 0)
	for _, e := range events {
		if e.Sequence > after && (eventType == "" || e.EventType == eventType) {
			filtered = append(filtered, e)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Sequence < filtered[j].Sequence })
	page := filtered
	if len(page) > limit {
		page = page[:limit]
	}
	result := ArchiveExport{Archive: m, StatusHistory: history, Events: page, ChainDigest: chainDigest, ExportDigest: exportDigest, ComplianceSummary: compliance, SummaryDigest: summaryDigest}
	if len(page) > 0 {
		result.SegmentStartDigest = page[0].PreviousHash
		result.SegmentEndDigest = page[len(page)-1].EventHash
	}
	if len(filtered) > limit {
		result.NextCursor = page[len(page)-1].Sequence
	}
	return result, nil
}
