package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

type Event struct {
	Sequence      int64           `json:"sequence"`
	MissionID     string          `json:"mission_id"`
	EventType     string          `json:"event_type"`
	ActorID       string          `json:"actor_id"`
	RequestID     string          `json:"request_id"`
	FromRevision  int64           `json:"from_revision"`
	ToRevision    int64           `json:"to_revision"`
	StatusAfter   string          `json:"status_after"`
	PayloadDigest string          `json:"payload_digest"`
	PreviousHash  string          `json:"previous_hash"`
	EventHash     string          `json:"event_hash"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Data          json.RawMessage `json:"data,omitempty"`
}

type Page struct {
	Events             []Event `json:"events"`
	NextCursor         int64   `json:"next_cursor,omitempty"`
	SegmentStartDigest string  `json:"segment_start_digest"`
	SegmentEndDigest   string  `json:"segment_end_digest"`
}

type Filter struct {
	After       int64
	Limit       int
	EventType   string
	StatusAfter string
}

func Digest(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func Build(missionID, eventType, actorID, requestID string, from, to int64, payload any, now time.Time) (Event, error) {
	digest, err := Digest(payload)
	if err != nil {
		return Event{}, err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	return Event{MissionID: missionID, EventType: eventType, ActorID: actorID, RequestID: requestID,
		FromRevision: from, ToRevision: to, PayloadDigest: digest, OccurredAt: now.UTC(), Data: data}, nil
}

func Seal(event Event, sequence int64, previousHash string) Event {
	event.Sequence = sequence
	event.PreviousHash = previousHash
	canonical := struct {
		Sequence      int64  `json:"sequence"`
		MissionID     string `json:"mission_id"`
		EventType     string `json:"event_type"`
		ActorID       string `json:"actor_id"`
		RequestID     string `json:"request_id"`
		FromRevision  int64  `json:"from_revision"`
		ToRevision    int64  `json:"to_revision"`
		StatusAfter   string `json:"status_after"`
		PayloadDigest string `json:"payload_digest"`
		PreviousHash  string `json:"previous_hash"`
		OccurredAt    string `json:"occurred_at"`
	}{sequence, event.MissionID, event.EventType, event.ActorID, event.RequestID, event.FromRevision,
		event.ToRevision, event.StatusAfter, event.PayloadDigest, previousHash, event.OccurredAt.UTC().Format(time.RFC3339Nano)}
	event.EventHash, _ = Digest(canonical)
	return event
}

func Verify(events []Event) error {
	previous := ""
	for i, event := range events {
		if event.Sequence != int64(i+1) {
			return errors.New("审计事件序号不连续")
		}
		if event.PreviousHash != previous {
			return errors.New("审计事件前序摘要不匹配")
		}
		payloadDigest, err := Digest(event.Data)
		if err != nil || payloadDigest != event.PayloadDigest {
			return errors.New("审计事件载荷摘要校验失败")
		}
		expected := Seal(event, event.Sequence, event.PreviousHash)
		if expected.EventHash != event.EventHash {
			return errors.New("审计事件摘要校验失败")
		}
		previous = event.EventHash
	}
	return nil
}

func ChainDigest(events []Event) (string, error) {
	if err := Verify(events); err != nil {
		return "", err
	}
	if len(events) == 0 {
		return Digest([]string{})
	}
	return events[len(events)-1].EventHash, nil
}
