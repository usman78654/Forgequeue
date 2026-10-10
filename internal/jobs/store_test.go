package jobs

import (
	"errors"
	"testing"
	"time"
)

func TestBackoffBounds(t *testing.T) {
	for attempt := 1; attempt <= 12; attempt++ {
		d := retryDelay(Job{Attempt: attempt, RetryPolicy: "exponential", RetryInitialSeconds: 2, RetryMaxSeconds: 300})
		base := time.Duration(2*1<<min(attempt-1, 8)) * time.Second
		if base > 300*time.Second {
			base = 300 * time.Second
		}
		if d != base {
			t.Fatalf("attempt %d: unexpected backoff %s", attempt, d)
		}
	}
}

func TestRetryPoliciesAndNonRetryableErrors(t *testing.T) {
	if got := retryDelay(Job{Attempt: 3, RetryPolicy: "fixed", RetryInitialSeconds: 4, RetryMaxSeconds: 60}); got != 4*time.Second {
		t.Fatalf("fixed=%s", got)
	}
	if got := retryDelay(Job{Attempt: 3, RetryPolicy: "linear", RetryInitialSeconds: 4, RetryMaxSeconds: 60}); got != 12*time.Second {
		t.Fatalf("linear=%s", got)
	}
	if !nonRetryable(Job{NonRetryableErrors: []string{"invalid input"}}, errors.New("Invalid Input: missing name")) {
		t.Fatal("expected error to be non-retryable")
	}
}

type permanentTestError struct{ error }

func (permanentTestError) Permanent() bool { return true }

func TestPermanentErrorIsNonRetryable(t *testing.T) {
	cause := errors.New("invalid typed payload")
	if !nonRetryable(Job{}, permanentTestError{cause}) {
		t.Fatal("expected permanent handler error to be non-retryable")
	}
	if nonRetryable(Job{}, cause) {
		t.Fatal("ordinary error should remain retryable")
	}
}
