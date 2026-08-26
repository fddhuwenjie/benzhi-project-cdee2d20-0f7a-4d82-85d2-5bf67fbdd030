package mission

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

type ArchiveIntegrityFilter struct {
	CaveSite string
	From, To *time.Time
	Limit    int
	Cursor   string
}

type ArchiveIntegrityItem struct {
	MissionID         string    `json:"mission_id"`
	CaveSite          string    `json:"cave_site"`
	ArchivedAt        time.Time `json:"archived_at"`
	Status            string    `json:"status"`
	FirstFailureLayer string    `json:"first_failure_layer,omitempty"`
	FailureSequence   int64     `json:"failure_sequence,omitempty"`
	CurrentDigest     string    `json:"current_digest,omitempty"`
}

type ArchiveIntegritySummary struct {
	Total                 int `json:"total"`
	Complete              int `json:"complete"`
	EventChainAnomaly     int `json:"event_chain_anomaly"`
	ArchiveDigestAnomaly  int `json:"archive_digest_anomaly"`
	SignedBaselineAnomaly int `json:"signed_baseline_anomaly"`
}

type ArchiveIntegrityResult struct {
	Items      []ArchiveIntegrityItem  `json:"items"`
	Summary    ArchiveIntegritySummary `json:"summary"`
	NextCursor string                  `json:"next_cursor,omitempty"`
	QueriedAt  time.Time               `json:"queried_at"`
}

type archiveCursor struct{ Site, From, To, ArchivedAt, ID string }

func encodeArchiveCursor(c archiveCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
func decodeArchiveCursor(v string) (archiveCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return archiveCursor{}, err
	}
	var c archiveCursor
	err = json.Unmarshal(b, &c)
	return c, err
}

func (s *Service) InspectArchiveIntegrity(ctx context.Context, f ArchiveIntegrityFilter) (ArchiveIntegrityResult, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		return ArchiveIntegrityResult{}, Invalid("limit", "必须在 1 到 200 之间")
	}
	if f.From != nil && f.To != nil && f.To.Before(*f.From) {
		return ArchiveIntegrityResult{}, Invalid("archived_to", "归档时间范围不能倒置")
	}
	var candidates []ArchiveCandidate
	var err error
	if lister, ok := s.repo.(ArchiveLister); ok {
		candidates, err = lister.ListArchived(ctx, ArchiveFilter{CaveSite: f.CaveSite, From: f.From, To: f.To})
	} else {
		listed, listErr := s.repo.List(ctx, ListFilter{Status: string(StatusArchived), CaveSite: f.CaveSite, Limit: 100})
		err = listErr
		for _, x := range listed.Items {
			m, e := s.repo.Mission(ctx, x.ID)
			if e != nil || m.ArchivedAt == nil || f.From != nil && m.ArchivedAt.Before(*f.From) || f.To != nil && m.ArchivedAt.After(*f.To) {
				continue
			}
			candidates = append(candidates, ArchiveCandidate{ID: m.ID, CaveSite: m.CaveSite, ArchivedAt: *m.ArchivedAt})
		}
	}
	if err != nil {
		return ArchiveIntegrityResult{}, err
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ArchivedAt.Equal(candidates[j].ArchivedAt) {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].ArchivedAt.Before(candidates[j].ArchivedAt)
	})
	start := 0
	if f.Cursor != "" {
		c, err := decodeArchiveCursor(f.Cursor)
		if err != nil || c.Site != strings.ToLower(strings.Join(strings.Fields(f.CaveSite), " ")) || c.From != archiveTime(f.From) || c.To != archiveTime(f.To) {
			return ArchiveIntegrityResult{}, Invalid("cursor", "游标与当前筛选条件不匹配")
		}
		found := false
		for i, x := range candidates {
			if archiveTime(&x.ArchivedAt) == c.ArchivedAt && x.ID == c.ID {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return ArchiveIntegrityResult{}, Invalid("cursor", "游标与当前筛选条件不匹配")
		}
	}
	result := ArchiveIntegrityResult{QueriedAt: s.now().UTC()}
	result.Summary.Total = len(candidates)
	for _, c := range candidates {
		item := s.inspectArchive(ctx, c)
		result.Items = append(result.Items, item)
		switch item.Status {
		case "complete":
			result.Summary.Complete++
		case "event_chain_anomaly":
			result.Summary.EventChainAnomaly++
		case "archive_digest_anomaly":
			result.Summary.ArchiveDigestAnomaly++
		case "signed_baseline_anomaly":
			result.Summary.SignedBaselineAnomaly++
		}
	}
	end := start + f.Limit
	if end > len(result.Items) {
		end = len(result.Items)
	}
	page := result.Items[start:end]
	result.Items = page
	if end < len(candidates) {
		last := candidates[end-1]
		result.NextCursor = encodeArchiveCursor(archiveCursor{Site: strings.ToLower(strings.Join(strings.Fields(f.CaveSite), " ")), From: archiveTime(f.From), To: archiveTime(f.To), ArchivedAt: archiveTime(&last.ArchivedAt), ID: last.ID})
	}
	return result, nil
}

func archiveTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func (s *Service) inspectArchive(ctx context.Context, c ArchiveCandidate) ArchiveIntegrityItem {
	item := ArchiveIntegrityItem{MissionID: c.ID, CaveSite: c.CaveSite, ArchivedAt: c.ArchivedAt, Status: "complete"}
	var m *DiveMission
	var err error
	if raw, ok := s.repo.(UnverifiedMissionReader); ok {
		m, err = raw.MissionUnverified(ctx, c.ID)
	} else {
		m, err = s.repo.Mission(ctx, c.ID)
	}
	if err != nil {
		item.Status, item.FirstFailureLayer = "event_chain_anomaly", "event_chain"
		return item
	}
	events, err := s.repo.AllEvents(ctx, c.ID)
	if err != nil {
		item.Status, item.FirstFailureLayer = "event_chain_anomaly", "event_chain"
		return item
	}
	if err := audit.Verify(events); err != nil {
		item.Status, item.FirstFailureLayer = "event_chain_anomaly", "event_chain"
		item.FailureSequence, item.CurrentDigest = firstBadEvent(events)
		return item
	}
	if len(events) < 2 || events[len(events)-1].EventType != "mission_archived" || events[len(events)-2].EventHash != m.ArchiveDigest {
		item.Status, item.FirstFailureLayer = "archive_digest_anomaly", "archive_digest"
		if len(events) > 0 {
			item.FailureSequence, item.CurrentDigest = events[len(events)-1].Sequence, events[len(events)-1].EventHash
		}
		return item
	}
	var archivedPayload struct {
		AuditChainDigest string `json:"audit_chain_digest"`
	}
	_ = json.Unmarshal(events[len(events)-1].Data, &archivedPayload)
	if archivedPayload.AuditChainDigest != m.ArchiveDigest {
		item.Status, item.FirstFailureLayer = "archive_digest_anomaly", "archive_digest"
		item.FailureSequence, item.CurrentDigest = events[len(events)-1].Sequence, events[len(events)-1].EventHash
		return item
	}
	signed := false
	for _, e := range events {
		if e.EventType != "mission_signed" {
			continue
		}
		signed = true
		var p struct {
			ReleaseDigest         string          `json:"release_digest"`
			Checklist             []ChecklistItem `json:"checklist"`
			RejectionConfirmation any             `json:"rejection_confirmation"`
		}
		_ = json.Unmarshal(e.Data, &p)
		tmp := *m
		tmp.Revision = e.ToRevision
		expected, digestErr := audit.Digest(releaseBaseline(&tmp, p.Checklist, p.RejectionConfirmation))
		if digestErr != nil || p.ReleaseDigest != m.ReleaseDigest || p.ReleaseDigest != expected || !reflect.DeepEqual(p.Checklist, m.ReleaseChecklist) {
			item.Status, item.FirstFailureLayer = "signed_baseline_anomaly", "signed_baseline"
			item.FailureSequence, item.CurrentDigest = e.Sequence, e.EventHash
			return item
		}
	}
	if !signed {
		item.Status, item.FirstFailureLayer = "signed_baseline_anomaly", "signed_baseline"
		return item
	}
	item.CurrentDigest = events[len(events)-1].EventHash
	return item
}

func firstBadEvent(events []audit.Event) (int64, string) {
	prev := ""
	for i, e := range events {
		payloadDigest, digestErr := audit.Digest(e.Data)
		expected := audit.Seal(e, e.Sequence, e.PreviousHash)
		if e.Sequence != int64(i+1) || e.PreviousHash != prev || digestErr != nil || payloadDigest != e.PayloadDigest || expected.EventHash != e.EventHash {
			return e.Sequence, e.EventHash
		}
		prev = e.EventHash
	}
	if len(events) > 0 {
		return events[len(events)-1].Sequence, events[len(events)-1].EventHash
	}
	return 0, ""
}
