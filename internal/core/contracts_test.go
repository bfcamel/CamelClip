package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
)

func TestWrappedFailuresRetainCodeAndRetryPolicy(t *testing.T) {
	e := fmt.Errorf("wrapped: %w", &Failure{Code: CodeConflict, Temporary: true})
	if CodeOf(e) != CodeConflict || !Retryable(e) {
		t.Fatal(e)
	}
	if Retryable(context.Canceled) {
		t.Fatal("cancellation retried")
	}
	if CodeOf(fmt.Errorf("wrap: %w", ErrCanceled)) != CodeCanceled {
		t.Fatal("cancellation code")
	}
	if !errors.Is(&Failure{Cause: context.Canceled}, context.Canceled) {
		t.Fatal("cause lost")
	}
}
func TestInvalidNumbersRejectedBeforePersistence(t *testing.T) {
	for _, n := range []float64{math.NaN(), math.Inf(1), -1} {
		if ValidateMediaInfo(MediaInfo{Duration: n}) == nil {
			t.Fatal(n)
		}
		if ValidateMetrics(Metrics{SpeedBPS: &n}) == nil {
			t.Fatal(n)
		}
	}
}
