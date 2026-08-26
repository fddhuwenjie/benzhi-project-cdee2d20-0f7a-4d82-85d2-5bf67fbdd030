package store

import (
	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func (s *Store) List(ctx context.Context, f mission.ListFilter) (mission.ListResult, error) {
	siteKey := strings.ToLower(strings.Join(strings.Fields(f.CaveSite), " "))
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM missions WHERE (?='' OR status=?) AND (?='' OR cave_site_key=?) AND (?='' OR leader_id=?) AND (?='' OR window_end>=?) AND (?='' OR window_start<=?)`, f.Status, f.Status, siteKey, siteKey, f.LeaderID, f.LeaderID, timeValue(f.WindowFrom), timeValue(f.WindowFrom), timeValue(f.WindowTo), timeValue(f.WindowTo))
	if err != nil {
		return mission.ListResult{}, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return mission.ListResult{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return mission.ListResult{}, err
	}
	if err := rows.Close(); err != nil {
		return mission.ListResult{}, err
	}
	var all []mission.MissionSummary
	for _, id := range ids {
		m, err := s.Mission(ctx, id)
		if err != nil {
			return mission.ListResult{}, err
		}
		mitigationState := mission.RiskMitigationState(m)
		if f.RiskLevel != "" && m.RiskSummary.HighestLevel != f.RiskLevel || m.RiskSummary.TotalScore < f.MinTotalScore || f.MitigationState != "" && mitigationState != f.MitigationState {
			continue
		}
		all = append(all, mission.MissionSummary{ID: m.ID, Title: m.Title, CaveSite: m.CaveSite, Status: m.Status, Revision: m.Revision, LeaderID: m.LeaderID, WindowStart: m.WindowStart, WindowEnd: m.WindowEnd, AllowedActions: mission.AllowedActions(m), HighestRisk: m.RiskSummary.HighestLevel, EquipmentComplete: mission.EquipmentComplete(m), TotalRiskScore: m.RiskSummary.TotalScore, MitigationState: mitigationState})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].WindowStart.Equal(all[j].WindowStart) {
			return all[i].ID < all[j].ID
		}
		return all[i].WindowStart.Before(all[j].WindowStart)
	})
	stats := mission.ListStatistics{StatusCounts: map[string]int{}, RiskHighest: map[string]int{"none": 0, "low": 0, "medium": 0, "high": 0, "critical": 0}, TotalScoreRanges: map[string]int{"0": 0, "1-9": 0, "10-19": 0, "20+": 0}, MitigationStates: map[string]int{"complete": 0, "incomplete": 0, "none": 0}}
	for _, status := range []mission.Status{mission.StatusDraft, mission.StatusRiskAssessed, mission.StatusPlanReview, mission.StatusEquipmentVerification, mission.StatusDrillPending, mission.StatusRemediation, mission.StatusReadyForRelease, mission.StatusReleaseRejected, mission.StatusSigned, mission.StatusArchived} {
		stats.StatusCounts[string(status)] = 0
	}
	for _, x := range all {
		stats.StatusCounts[string(x.Status)]++
		if x.HighestRisk != "" {
			stats.RiskHighest[x.HighestRisk]++
		} else {
			stats.RiskHighest["none"]++
		}
		stats.MitigationStates[x.MitigationState]++
		switch {
		case x.TotalRiskScore == 0:
			stats.TotalScoreRanges["0"]++
		case x.TotalRiskScore < 10:
			stats.TotalScoreRanges["1-9"]++
		case x.TotalRiskScore < 20:
			stats.TotalScoreRanges["10-19"]++
		default:
			stats.TotalScoreRanges["20+"]++
		}
		if x.Status == mission.StatusRemediation {
			stats.RemediationCount++
		}
		if !x.EquipmentComplete {
			stats.EquipmentIncompleteCount++
		}
	}
	start := 0
	if f.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		if e != nil {
			return mission.ListResult{}, mission.Invalid("cursor", "游标格式无效")
		}
		var cursor struct {
			WindowStart string `json:"window_start"`
			ID          string `json:"id"`
		}
		e = json.Unmarshal(b, &cursor)
		if e != nil || cursor.WindowStart == "" || cursor.ID == "" {
			return mission.ListResult{}, mission.Invalid("cursor", "游标格式无效")
		}
		found := false
		for i, item := range all {
			if stamp(item.WindowStart) == cursor.WindowStart && item.ID == cursor.ID {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return mission.ListResult{}, mission.Invalid("cursor", "游标与当前筛选条件不匹配")
		}
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	end := start + limit
	if end > len(all) {
		end = len(all)
	}
	res := mission.ListResult{Items: all[start:end], Statistics: stats}
	if end < len(all) {
		last := all[end-1]
		b, _ := json.Marshal(struct {
			WindowStart string `json:"window_start"`
			ID          string `json:"id"`
		}{stamp(last.WindowStart), last.ID})
		res.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
	return res, nil
}
func timeValue(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
