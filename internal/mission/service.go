package mission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service { return &Service{repo: repo, now: time.Now} }

func newID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func resultFor(m *DiveMission, status int) (StoredResult, error) {
	body, err := json.Marshal(CommandResult{Mission: m, AllowedActions: AllowedActions(m)})
	return StoredResult{StatusCode: status, Body: body}, err
}

func resultForPayload(m *DiveMission, status int, payload any) (StoredResult, error) {
	base := map[string]any{"mission": m, "allowed_actions": AllowedActions(m)}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return StoredResult{}, err
		}
		var fields map[string]any
		if json.Unmarshal(raw, &fields) == nil {
			for key, value := range fields {
				if key != "mission" && key != "allowed_actions" {
					base[key] = value
				}
			}
		} else {
			base["result"] = payload
		}
	}
	body, err := json.Marshal(base)
	return StoredResult{StatusCode: status, Body: body}, err
}

func commandFingerprint(operation string, meta CommandMeta, payload any) (string, error) {
	return audit.Digest(struct {
		Operation string      `json:"operation"`
		Meta      CommandMeta `json:"meta"`
		Payload   any         `json:"payload"`
	}{operation, meta, payload})
}

func (s *Service) command(ctx context.Context, missionID, operation string, meta CommandMeta, payload any, mutate func(context.Context, Tx, *DiveMission) (string, any, error)) (StoredResult, bool, error) {
	if err := ValidateMeta(meta); err != nil {
		return StoredResult{}, false, err
	}
	fingerprint, err := commandFingerprint(operation, meta, payload)
	if err != nil {
		return StoredResult{}, false, err
	}
	return s.repo.Execute(ctx, missionID, meta.RequestID, operation, fingerprint, func(tx Tx) (StoredResult, error) {
		m, err := tx.LoadMission(ctx, missionID)
		if err != nil {
			return StoredResult{}, err
		}
		if m.Status == StatusArchived {
			return StoredResult{}, ErrArchived
		}
		if m.Revision != meta.ExpectedRevision {
			events, eventsErr := tx.AllEvents(ctx, missionID)
			if eventsErr != nil {
				return StoredResult{}, eventsErr
			}
			last := ""
			if len(events) > 0 {
				last = events[len(events)-1].EventType
			}
			return StoredResult{}, &Error{Code: "revision_conflict", Message: "任务修订号冲突", Status: 409, Details: map[string]any{
				"current_revision": m.Revision, "recent_event_type": last, "retry_action": "reload_and_retry",
			}}
		}
		before := m.Revision
		eventType, payload, err := mutate(ctx, tx, m)
		if err != nil {
			return StoredResult{}, err
		}
		m.Revision++
		m.UpdatedAt = s.now().UTC()
		if err := tx.SaveMission(ctx, m, before); err != nil {
			return StoredResult{}, err
		}
		event, err := audit.Build(m.ID, eventType, meta.ActorID, meta.RequestID, before, m.Revision, payload, s.now())
		if err != nil {
			return StoredResult{}, err
		}
		event.StatusAfter = string(m.Status)
		if _, err = tx.AppendEvent(ctx, event); err != nil {
			return StoredResult{}, err
		}
		return resultForPayload(m, 200, payload)
	})
}

func (s *Service) Mission(ctx context.Context, id string) (*DiveMission, error) {
	m, err := s.repo.Mission(ctx, id)
	if err != nil {
		return nil, err
	}
	m.CycleStatuses = cycleStatuses(m)
	m.RemediationDeadlines = projectRemediationDeadlines(m, s.now().UTC())
	m.RiskMitigationBlockers = mitigationBlockers(m, s.now().UTC())
	return m, nil
}
func (s *Service) History(ctx context.Context, id string) ([]HistoryEntry, error) {
	return s.repo.History(ctx, id)
}
func (s *Service) List(ctx context.Context, f ListFilter) (ListResult, error) {
	return s.repo.List(ctx, f)
}

func AsError(err error) *Error {
	var target *Error
	if errors.As(err, &target) {
		return target
	}
	return &Error{Code: "internal_error", Message: "服务内部错误", Status: 500}
}
