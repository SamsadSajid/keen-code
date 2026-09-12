package retry

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	openai "github.com/openai/openai-go/v3"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}

func TestCount(t *testing.T) {
	if Count(0) != 6 || Count(-1) != 6 || Count(3) != 3 {
		t.Fatal("unexpected retry counts")
	}
}

func TestRetryable(t *testing.T) {
	tests := []struct {
		err       error
		retryable bool
	}{
		{err: &openai.Error{StatusCode: http.StatusTooManyRequests}, retryable: true},
		{err: &openai.Error{StatusCode: http.StatusInternalServerError}, retryable: true},
		{err: &openai.Error{StatusCode: http.StatusBadRequest}, retryable: false},
		{err: timeoutError{}, retryable: true},
		{err: context.Canceled, retryable: false},
		{err: context.DeadlineExceeded, retryable: false},
		{err: errors.New("other"), retryable: true},
	}
	for _, tt := range tests {
		if got := retryable(tt.err); got != tt.retryable {
			t.Errorf("retryable(%v) = %v, want %v", tt.err, got, tt.retryable)
		}
	}
}

func TestRunRetriesAndReports(t *testing.T) {
	attempts := 0
	var reported []int
	started := time.Now()
	err := Run(context.Background(), 2, func(attempt, maxRetries int, err error) {
		reported = append(reported, attempt)
	}, func() error {
		attempts++
		if attempts == 1 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil || attempts != 2 || len(reported) != 1 || reported[0] != 1 {
		t.Fatalf("unexpected result: err=%v attempts=%d reported=%v", err, attempts, reported)
	}
	if time.Since(started) < time.Second {
		t.Fatal("expected retry backoff")
	}
}

func TestRunStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	attempts := 0
	err := Run(ctx, 2, nil, func() error {
		attempts++
		return errors.New("temporary")
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("unexpected result: err=%v attempts=%d", err, attempts)
	}
}
