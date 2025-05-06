// pkg/backoff/backoff_test.go
package backoff_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/backoff"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
)

func TestExecute_SuccessFirstAttempt(t *testing.T) {
	cfg := backoff.Config{MaxElapsedTime: time.Second}
	log, _ := logger.New(logger.Config{Level: "debug", DevMode: true})
	called := 0
	err := backoff.Execute(context.Background(), cfg, log, func(ctx context.Context) error {
		called++
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if called != 1 {
		t.Errorf("expected 1 attempt, got %d", called)
	}
}

func TestExecute_EventualSuccess(t *testing.T) {
	cfg := backoff.Config{InitialInterval: 10 * time.Millisecond, Multiplier: 1, MaxElapsedTime: 100 * time.Millisecond}
	log, _ := logger.New(logger.Config{Level: "debug", DevMode: true})
	attemptsBeforeSuccess := 3
	called := 0
	err := backoff.Execute(context.Background(), cfg, log, func(ctx context.Context) error {
		called++
		if called < attemptsBeforeSuccess {
			return errors.New("fail")
		}
		return nil
	})
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
	if called != attemptsBeforeSuccess {
		t.Errorf("expected %d attempts, got %d", attemptsBeforeSuccess, called)
	}
}

func TestExecute_MaxRetriesExceeded(t *testing.T) {
	cfg := backoff.Config{InitialInterval: 10 * time.Millisecond, Multiplier: 1, MaxElapsedTime: 50 * time.Millisecond}
	log, _ := logger.New(logger.Config{Level: "debug", DevMode: true})
	called := 0
	err := backoff.Execute(context.Background(), cfg, log, func(ctx context.Context) error {
		called++
		return errors.New("always fail")
	})
	var maxErr *backoff.ErrMaxRetries
	if !errors.As(err, &maxErr) {
		t.Fatalf("expected ErrMaxRetries, got %v", err)
	}
	if maxErr.Attempts != called {
		t.Errorf("attempts mismatch: ErrMaxRetries.Attempts=%d, actual=%d", maxErr.Attempts, called)
	}
}
