package archiveinspectioncancellation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

type canceledArchiveRepo struct {
	mission.Repository
}

func (r *canceledArchiveRepo) ListArchived(context.Context, mission.ArchiveFilter) ([]mission.ArchiveCandidate, error) {
	return []mission.ArchiveCandidate{{ID: "archived-mission", CaveSite: "洞穴", ArchivedAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}}, nil
}

func (r *canceledArchiveRepo) Mission(ctx context.Context, _ string) (*mission.DiveMission, error) {
	return nil, ctx.Err()
}

func TestArchiveInspectionPropagatesCancellationBetweenBatchReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := mission.NewService(&canceledArchiveRepo{}).InspectArchiveIntegrity(ctx, mission.ArchiveIntegrityFilter{Limit: 10})
	if errors.Is(err, context.Canceled) {
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	t.Fatalf("TestArchiveInspectionPropagatesCancellationBetweenBatchReads: canceled repository read was converted to status %q at layer %q", result.Items[0].Status, result.Items[0].FirstFailureLayer)
}
