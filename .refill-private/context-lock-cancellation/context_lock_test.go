package contextlockcancellation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/store"
)

type executeResult struct {
	err error
}

func TestCanceledExecuteDoesNotWaitForUnrelatedTransaction(t *testing.T) {
	repository, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, _, executeErr := repository.Execute(context.Background(), "mission-a", "request-a", "hold", "a", func(mission.Tx) (mission.StoredResult, error) {
			close(entered)
			<-release
			return mission.StoredResult{StatusCode: 200, Body: []byte(`{"ok":true}`)}, nil
		})
		firstDone <- executeErr
	}()
	<-entered

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	started := make(chan struct{})
	secondDone := make(chan executeResult, 1)
	go func() {
		close(started)
		_, _, executeErr := repository.Execute(canceled, "mission-b", "request-b", "write", "b", func(mission.Tx) (mission.StoredResult, error) {
			return mission.StoredResult{}, errors.New("canceled callback must not run")
		})
		secondDone <- executeResult{err: executeErr}
	}()
	<-started

	select {
	case result := <-secondDone:
		close(release)
		if firstErr := <-firstDone; firstErr != nil {
			t.Fatal(firstErr)
		}
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", result.err)
		}
	case <-time.After(250 * time.Millisecond):
		close(release)
		if firstErr := <-firstDone; firstErr != nil {
			t.Fatal(firstErr)
		}
		result := <-secondDone
		t.Fatalf("TestCanceledExecuteDoesNotWaitForUnrelatedTransaction: canceled request waited on the store transaction mutex; eventual error: %v", result.err)
	}
}
