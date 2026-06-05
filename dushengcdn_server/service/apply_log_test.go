package service

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"dushengcdn/model"
)

func TestLoadApplyLogPageDataRunsQueriesConcurrently(t *testing.T) {
	const queryCount = 2
	started := make(chan struct{}, queryCount)
	release := make(chan struct{})
	var calls atomic.Int32

	markStartedAndWait := func() {
		calls.Add(1)
		started <- struct{}{}
		<-release
	}

	queries := applyLogPageQueries{
		listApplyLogs: func(model.ApplyLogQuery) ([]*model.ApplyLog, error) {
			markStartedAndWait()
			return []*model.ApplyLog{{NodeID: "node-apply"}}, nil
		},
		countApplyLogs: func(string) (int64, error) {
			markStartedAndWait()
			return 1, nil
		},
	}

	done := make(chan error, 1)
	go func() {
		rows, total, err := loadApplyLogPageData("node-apply", 1, 20, queries)
		if err == nil && (len(rows) != 1 || total != 1) {
			err = errors.New("unexpected apply log page data")
		}
		done <- err
	}()

	for index := 0; index < queryCount; index++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatalf("expected all apply log page queries to start concurrently, got %d/%d", calls.Load(), queryCount)
		}
	}
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loadApplyLogPageData returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for concurrent apply log page query load")
	}
}

func TestLoadApplyLogPageDataReturnsQueryError(t *testing.T) {
	wantErr := errors.New("apply log list failed")
	queries := applyLogPageQueries{
		listApplyLogs: func(model.ApplyLogQuery) ([]*model.ApplyLog, error) {
			return nil, wantErr
		},
		countApplyLogs: func(string) (int64, error) {
			return 0, nil
		},
	}

	_, _, err := loadApplyLogPageData("node-apply", 1, 20, queries)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected query error to be returned, got %v", err)
	}
}
