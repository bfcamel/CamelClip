package application

import (
	"context"
	"testing"
	"time"
)

func TestRetryResetsCurrentAttemptWithoutLosingExecutionCount(t *testing.T) {
	j := Job{ID: 1, Request: req(), State: Failed, OutputPath: "/previous.mp4", Monitoring: Monitoring{AttemptID: "old", Executions: 3, AutomaticRetries: 2, LastError: "old failure", ErrorCode: "backend", FinishedAt: time.Now()}}
	s, e := New(context.Background(), &fakeDownloader{}, &memoryRepository{snapshot: Snapshot{NextID: 1, Jobs: []Job{j}}}, 1)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Retry(1, ""); e != nil {
		t.Fatal(e)
	}
	got := s.Jobs()[0]
	if got.OutputPath != "" || got.Monitoring.LastError != "" || got.Monitoring.ErrorCode != "" || got.Monitoring.AttemptID != "" || got.Monitoring.AutomaticRetries != 0 || got.Monitoring.Executions != 3 || !got.Monitoring.FinishedAt.IsZero() {
		t.Fatal(got)
	}
}
