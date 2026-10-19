package kafka

import (
	"testing"
	"time"
)

func TestOnlyAttemptsWithoutProgressWait(t *testing.T) {
	if got := nextAttempt(3, true); got != 0 {
		t.Errorf("nextAttempt after progress = %d, want 0", got)
	}
	if got := nextAttempt(3, false); got != 4 {
		t.Errorf("nextAttempt without progress = %d, want 4", got)
	}
}

func TestReplayDelayDoublesUpToACeilingWithJitter(t *testing.T) {
	for attempt, base := range map[int]time.Duration{0: time.Second, 1: 2 * time.Second, 4: 16 * time.Second, 5: 30 * time.Second, 40: 30 * time.Second} {
		for range 50 {
			d := replayDelay(attempt)
			if d < base*8/10 || d > base*12/10 {
				t.Fatalf("replayDelay(%d) = %v, want within 20 %% of %v", attempt, d, base)
			}
		}
	}
}
