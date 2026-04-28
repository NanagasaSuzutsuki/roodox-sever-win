package cleanup

import (
	"testing"
	"time"
)

func TestRunnerTriggerExecutesImmediatelyAndSchedulesNextDue(t *testing.T) {
	callCh := make(chan time.Time, 4)
	callCount := 0

	runner := NewRunner(0, func(now time.Time) time.Time {
		callCount++
		callCh <- now
		if callCount == 2 {
			return now.Add(40 * time.Millisecond)
		}
		return time.Time{}
	})
	defer runner.Close()

	waitForRunnerCall(t, callCh, 200*time.Millisecond)

	triggeredAt := time.Now()
	runner.Trigger()
	secondCall := waitForRunnerCall(t, callCh, 200*time.Millisecond)
	if secondCall.Sub(triggeredAt) > 50*time.Millisecond {
		t.Fatalf("triggered cleanup took too long: %v", secondCall.Sub(triggeredAt))
	}

	thirdCall := waitForRunnerCall(t, callCh, 300*time.Millisecond)
	if thirdCall.Sub(secondCall) < 30*time.Millisecond {
		t.Fatalf("scheduled cleanup ran too early: %v", thirdCall.Sub(secondCall))
	}
}

func TestRunnerCoalescesTriggersWithinInterval(t *testing.T) {
	callCh := make(chan time.Time, 4)

	runner := NewRunner(60*time.Millisecond, func(now time.Time) time.Time {
		callCh <- now
		return time.Time{}
	})
	defer runner.Close()

	firstCall := waitForRunnerCall(t, callCh, 200*time.Millisecond)

	runner.Trigger()
	runner.Trigger()

	secondCall := waitForRunnerCall(t, callCh, 300*time.Millisecond)
	if secondCall.Sub(firstCall) < 45*time.Millisecond {
		t.Fatalf("throttled cleanup ran too early: %v", secondCall.Sub(firstCall))
	}

	select {
	case extra := <-callCh:
		t.Fatalf("expected coalesced trigger, got extra cleanup at %v", extra)
	case <-time.After(120 * time.Millisecond):
	}
}

func waitForRunnerCall(t *testing.T, callCh <-chan time.Time, timeout time.Duration) time.Time {
	t.Helper()

	select {
	case ts := <-callCh:
		return ts
	case <-time.After(timeout):
		t.Fatalf("runner did not execute within %v", timeout)
		return time.Time{}
	}
}
