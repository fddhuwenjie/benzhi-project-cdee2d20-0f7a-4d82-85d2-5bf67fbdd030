package audit

import "context"

type Reader interface {
	AllAuditEvents(context.Context, string) ([]Event, error)
}

type Service struct{ reader Reader }

func NewService(reader Reader) *Service { return &Service{reader: reader} }

func (s *Service) Page(ctx context.Context, missionID string, filter Filter) (Page, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	events, err := s.reader.AllAuditEvents(ctx, missionID)
	if err != nil {
		return Page{}, err
	}
	if err := Verify(events); err != nil {
		return Page{}, err
	}
	page := Page{}
	for _, event := range events {
		if event.Sequence <= filter.After || filter.EventType != "" && event.EventType != filter.EventType || filter.StatusAfter != "" && event.StatusAfter != filter.StatusAfter {
			continue
		}
		if len(page.Events) == filter.Limit {
			page.NextCursor = page.Events[len(page.Events)-1].Sequence
			break
		}
		page.Events = append(page.Events, event)
	}
	if len(page.Events) > 0 {
		page.SegmentStartDigest = page.Events[0].PreviousHash
		page.SegmentEndDigest = page.Events[len(page.Events)-1].EventHash
	}
	return page, nil
}

func (s *Service) Verify(ctx context.Context, missionID string) (string, error) {
	events, err := s.reader.AllAuditEvents(ctx, missionID)
	if err != nil {
		return "", err
	}
	return ChainDigest(events)
}
