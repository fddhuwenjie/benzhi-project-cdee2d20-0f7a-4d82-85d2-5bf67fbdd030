package store

import (
	"context"
	"database/sql"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func (t *transaction) AppendEvent(ctx context.Context, event audit.Event) (audit.Event, error) {
	var sequence int64
	var previous string
	err := t.tx.QueryRowContext(ctx, `SELECT sequence,event_hash FROM audit_events WHERE mission_id=? ORDER BY sequence DESC LIMIT 1`, event.MissionID).Scan(&sequence, &previous)
	if err == sql.ErrNoRows {
		sequence = 0
		previous = ""
	} else if err != nil {
		return audit.Event{}, err
	}
	event = audit.Seal(event, sequence+1, previous)
	_, err = t.tx.ExecContext(ctx, `INSERT INTO audit_events(mission_id,sequence,event_type,actor_id,request_id,from_revision,to_revision,status_after,payload_digest,previous_hash,event_hash,occurred_at,data) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, event.MissionID, event.Sequence, event.EventType, event.ActorID, event.RequestID, event.FromRevision, event.ToRevision, event.StatusAfter, event.PayloadDigest, event.PreviousHash, event.EventHash, stamp(event.OccurredAt), []byte(event.Data))
	return event, err
}

func (t *transaction) AllEvents(ctx context.Context, id string) ([]audit.Event, error) {
	return readEvents(ctx, t.tx, id, 0, 100000)
}
func (s *Store) AllAuditEvents(ctx context.Context, id string) ([]audit.Event, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM missions WHERE id=?`, id).Scan(&exists); err == sql.ErrNoRows {
		return nil, mission.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return readEvents(ctx, s.db, id, 0, 100000)
}

func (s *Store) AllEvents(ctx context.Context, id string) ([]audit.Event, error) {
	return s.AllAuditEvents(ctx, id)
}

func (s *Store) AuditEvents(ctx context.Context, id string, after int64, limit int) (audit.Page, error) {
	events, err := readEvents(ctx, s.db, id, after, limit+1)
	if err != nil {
		return audit.Page{}, err
	}
	page := audit.Page{Events: events}
	if len(events) > limit {
		page.Events = events[:limit]
		page.NextCursor = events[limit-1].Sequence
	}
	return page, nil
}

func readEvents(ctx context.Context, q sqlRunner, id string, after int64, limit int) ([]audit.Event, error) {
	rows, err := q.QueryContext(ctx, `SELECT sequence,event_type,actor_id,request_id,from_revision,to_revision,status_after,payload_digest,previous_hash,event_hash,occurred_at,data FROM audit_events WHERE mission_id=? AND sequence>? ORDER BY sequence LIMIT ?`, id, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []audit.Event
	for rows.Next() {
		var e audit.Event
		var occurred string
		e.MissionID = id
		if err := rows.Scan(&e.Sequence, &e.EventType, &e.ActorID, &e.RequestID, &e.FromRevision, &e.ToRevision, &e.StatusAfter, &e.PayloadDigest, &e.PreviousHash, &e.EventHash, &occurred, &e.Data); err != nil {
			return nil, err
		}
		e.OccurredAt, err = time.Parse(time.RFC3339Nano, occurred)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func verifyArchive(m *mission.DiveMission, events []audit.Event) error {
	if err := audit.Verify(events); err != nil {
		return mission.NewError("archive_integrity_failed", "归档审计链校验失败", 409)
	}
	if len(events) < 2 || events[len(events)-1].EventType != "mission_archived" || events[len(events)-2].EventHash != m.ArchiveDigest {
		return mission.NewError("archive_integrity_failed", "归档摘要与审计链不一致", 409)
	}
	return nil
}

func (s *Store) History(ctx context.Context, id string) ([]mission.HistoryEntry, error) {
	if _, err := s.Mission(ctx, id); err != nil {
		return nil, err
	}
	events, err := s.AllAuditEvents(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]mission.HistoryEntry, 0, len(events))
	for _, event := range events {
		result = append(result, mission.HistoryEntry{Status: mission.Status(event.StatusAfter), Revision: event.ToRevision, EventType: event.EventType, OccurredAt: event.OccurredAt})
	}
	return result, nil
}
