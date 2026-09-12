package retry

import (
	"context"
	"errors"
	"net/http"
	"time"

	openai "github.com/openai/openai-go/v3"
)

const defaultMaxRetries = 6

func Count(maxRetries int) int {
	if maxRetries <= 0 {
		return defaultMaxRetries
	}
	return maxRetries
}

func retryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= http.StatusInternalServerError
	}
	return true
}

func Run(ctx context.Context, maxRetries int, onRetry func(int, int, error), run func() error) error {
	maxRetries = Count(maxRetries)
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := run()
		if err == nil || !retryable(err) || attempt == maxRetries {
			return err
		}
		if onRetry != nil {
			onRetry(attempt, maxRetries, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
	return nil
}
