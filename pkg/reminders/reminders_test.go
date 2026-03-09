package reminders

import (
	"testing"
	"time"
)

func TestNormalizePolicyAppliesDefaultsAndBounds(t *testing.T) {
	out := NormalizePolicy(Policy{Enabled: true, JitterPercent: 999, MaxCount: -1, InitialDelay: -1, Interval: 0})
	if out.InitialDelay <= 0 {
		t.Fatalf("expected default initial delay")
	}
	if out.Interval <= 0 {
		t.Fatalf("expected default interval")
	}
	if out.MaxCount <= 0 {
		t.Fatalf("expected default max count")
	}
	if out.JitterPercent != maxJitterPercent {
		t.Fatalf("expected jitter percent clamp to %d, got %d", maxJitterPercent, out.JitterPercent)
	}
}

func TestEvaluateDueAndNotDueBoundaries(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	policy := Policy{
		Enabled:              true,
		InitialDelay:         24 * time.Hour,
		Interval:             24 * time.Hour,
		MaxCount:             5,
		JitterPercent:        0,
		RecentViewGrace:      12 * time.Hour,
		ManualResendCooldown: 24 * time.Hour,
	}
	next := now.Add(2 * time.Hour)
	d := Evaluate(now, policy, State{NextDueAt: &next})
	if d.Due {
		t.Fatalf("expected not due")
	}
	if d.ReasonCode != ReasonNotDueYet {
		t.Fatalf("expected %s, got %s", ReasonNotDueYet, d.ReasonCode)
	}

	next = now.Add(-1 * time.Minute)
	d = Evaluate(now, policy, State{NextDueAt: &next})
	if !d.Due {
		t.Fatalf("expected due")
	}
	if d.ReasonCode != ReasonDue {
		t.Fatalf("expected %s, got %s", ReasonDue, d.ReasonCode)
	}
}

func TestEvaluateCooldownPrecedence(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	policy := NormalizePolicy(Policy{Enabled: true})
	lastManual := now.Add(-1 * time.Hour)
	lastView := now.Add(-30 * time.Minute)
	d := Evaluate(now, policy, State{LastManualResendAt: &lastManual, LastViewedAt: &lastView})
	if d.Due {
		t.Fatalf("expected blocked")
	}
	if d.ReasonCode != ReasonManualResendCooldown {
		t.Fatalf("expected %s, got %s", ReasonManualResendCooldown, d.ReasonCode)
	}
}

func TestEvaluateMaxCount(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	policy := NormalizePolicy(Policy{Enabled: true, MaxCount: 2})
	d := Evaluate(now, policy, State{SentCount: 2})
	if d.Due {
		t.Fatalf("expected blocked at max")
	}
	if d.ReasonCode != ReasonMaxCountReached {
		t.Fatalf("expected %s, got %s", ReasonMaxCountReached, d.ReasonCode)
	}
}

func TestComputeNextDueDeterministic(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	policy := NormalizePolicy(Policy{Enabled: true, Interval: 24 * time.Hour, JitterPercent: 20})
	a := ComputeNextDue(now, policy, "tenant|org|agreement|recipient")
	b := ComputeNextDue(now, policy, "tenant|org|agreement|recipient")
	if !a.Equal(b) {
		t.Fatalf("expected deterministic next due, got %s and %s", a, b)
	}
	c := ComputeNextDue(now, policy, "different")
	if a.Equal(c) {
		t.Fatalf("expected jitter to differ for different stable keys")
	}
}

func TestPolicyAndStateMapRoundTrip(t *testing.T) {
	policy := NormalizePolicy(Policy{Enabled: true, Interval: 6 * time.Hour, InitialDelay: 30 * time.Minute, MaxCount: 9, JitterPercent: 12})
	pm := PolicyToMap(policy)
	decodedPolicy, err := PolicyFromMap(pm)
	if err != nil {
		t.Fatalf("PolicyFromMap: %v", err)
	}
	if NormalizePolicy(decodedPolicy).Interval != policy.Interval {
		t.Fatalf("interval mismatch")
	}

	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	state := State{SentCount: 2, LastSentAt: &now, NextDueAt: &now}
	sm := StateToMap(state)
	decodedState, err := StateFromMap(sm)
	if err != nil {
		t.Fatalf("StateFromMap: %v", err)
	}
	if decodedState.SentCount != state.SentCount {
		t.Fatalf("sent count mismatch")
	}
	if decodedState.LastSentAt == nil || !decodedState.LastSentAt.Equal(now) {
		t.Fatalf("last sent mismatch")
	}
}
