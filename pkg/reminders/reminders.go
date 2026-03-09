package reminders

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"strings"
	"time"
)

const (
	ReasonDue                  = "due"
	ReasonDisabled             = "disabled"
	ReasonMaxCountReached      = "max_count_reached"
	ReasonRecentViewGrace      = "recent_view_grace"
	ReasonManualResendCooldown = "manual_resend_cooldown"
	ReasonNotDueYet            = "not_due_yet"
)

const (
	defaultInitialDelay         = 24 * time.Hour
	defaultInterval             = 24 * time.Hour
	defaultMaxCount             = 5
	defaultJitterPercent        = 15
	defaultRecentViewGrace      = 12 * time.Hour
	defaultManualResendCooldown = 24 * time.Hour
	maxJitterPercent            = 90
)

// Policy defines reminder cadence and anti-spam constraints.
type Policy struct {
	Enabled              bool
	InitialDelay         time.Duration
	Interval             time.Duration
	MaxCount             int
	JitterPercent        int
	RecentViewGrace      time.Duration
	ManualResendCooldown time.Duration
}

// State tracks reminder execution and recipient behavior signals.
type State struct {
	SentCount          int
	FirstSentAt        *time.Time
	LastSentAt         *time.Time
	LastViewedAt       *time.Time
	LastManualResendAt *time.Time
	NextDueAt          *time.Time
}

// Decision is the evaluation output for a reminder candidate.
type Decision struct {
	Due        bool
	ReasonCode string
	NextDueAt  *time.Time
}

// NormalizePolicy applies defaults and clamps unsafe values.
func NormalizePolicy(in Policy) Policy {
	out := in
	if out.InitialDelay <= 0 {
		out.InitialDelay = defaultInitialDelay
	}
	if out.Interval <= 0 {
		out.Interval = defaultInterval
	}
	if out.MaxCount <= 0 {
		out.MaxCount = defaultMaxCount
	}
	if out.JitterPercent < 0 {
		out.JitterPercent = 0
	}
	if out.JitterPercent == 0 {
		out.JitterPercent = defaultJitterPercent
	}
	if out.JitterPercent > maxJitterPercent {
		out.JitterPercent = maxJitterPercent
	}
	if out.RecentViewGrace < 0 {
		out.RecentViewGrace = 0
	}
	if out.RecentViewGrace == 0 {
		out.RecentViewGrace = defaultRecentViewGrace
	}
	if out.ManualResendCooldown < 0 {
		out.ManualResendCooldown = 0
	}
	if out.ManualResendCooldown == 0 {
		out.ManualResendCooldown = defaultManualResendCooldown
	}
	return out
}

// Evaluate determines whether a reminder is due and returns the next due boundary.
func Evaluate(now time.Time, policy Policy, state State) Decision {
	now = normalizeTime(now)
	resolved := NormalizePolicy(policy)
	if !resolved.Enabled {
		return Decision{Due: false, ReasonCode: ReasonDisabled}
	}
	if resolved.MaxCount > 0 && state.SentCount >= resolved.MaxCount {
		return Decision{Due: false, ReasonCode: ReasonMaxCountReached}
	}

	if blockedUntil, ok := blockUntil(state.LastManualResendAt, resolved.ManualResendCooldown); ok && now.Before(blockedUntil) {
		return Decision{Due: false, ReasonCode: ReasonManualResendCooldown, NextDueAt: cloneTimePtr(&blockedUntil)}
	}
	if blockedUntil, ok := blockUntil(state.LastViewedAt, resolved.RecentViewGrace); ok && now.Before(blockedUntil) {
		return Decision{Due: false, ReasonCode: ReasonRecentViewGrace, NextDueAt: cloneTimePtr(&blockedUntil)}
	}

	dueAt := deriveDueAt(now, resolved, state)
	if dueAt.After(now) {
		return Decision{Due: false, ReasonCode: ReasonNotDueYet, NextDueAt: cloneTimePtr(&dueAt)}
	}
	return Decision{Due: true, ReasonCode: ReasonDue, NextDueAt: cloneTimePtr(&dueAt)}
}

// ComputeNextDue calculates deterministic next due time with jitter around the interval.
func ComputeNextDue(now time.Time, policy Policy, stableKey string) time.Time {
	now = normalizeTime(now)
	resolved := NormalizePolicy(policy)
	base := now.Add(resolved.Interval)
	jitter := deterministicJitter(resolved.Interval, resolved.JitterPercent, stableKey)
	candidate := base.Add(jitter)
	// Do not schedule in the past relative to now.
	if candidate.Before(now) {
		return now
	}
	return candidate.UTC()
}

func deriveDueAt(now time.Time, policy Policy, state State) time.Time {
	if state.NextDueAt != nil && !state.NextDueAt.IsZero() {
		return state.NextDueAt.UTC()
	}
	if state.SentCount <= 0 {
		if state.FirstSentAt != nil && !state.FirstSentAt.IsZero() {
			return state.FirstSentAt.UTC().Add(policy.InitialDelay)
		}
		return now.Add(policy.InitialDelay)
	}
	if state.LastSentAt != nil && !state.LastSentAt.IsZero() {
		return state.LastSentAt.UTC().Add(policy.Interval)
	}
	if state.FirstSentAt != nil && !state.FirstSentAt.IsZero() {
		return state.FirstSentAt.UTC().Add(policy.Interval)
	}
	return now.Add(policy.Interval)
}

func blockUntil(at *time.Time, window time.Duration) (time.Time, bool) {
	if at == nil || at.IsZero() || window <= 0 {
		return time.Time{}, false
	}
	until := at.UTC().Add(window)
	return until, true
}

func deterministicJitter(base time.Duration, percent int, stableKey string) time.Duration {
	if base <= 0 || percent <= 0 {
		return 0
	}
	stableKey = strings.TrimSpace(stableKey)
	if stableKey == "" {
		return 0
	}
	sum := sha256.Sum256([]byte(stableKey))
	value := binary.BigEndian.Uint64(sum[:8])
	ratio := float64(value) / float64(math.MaxUint64) // [0,1]
	signed := (ratio * 2.0) - 1.0                     // [-1,1]
	maxAbs := float64(base) * (float64(percent) / 100.0)
	offset := signed * maxAbs
	return time.Duration(offset)
}

func normalizeTime(in time.Time) time.Time {
	if in.IsZero() {
		return time.Now().UTC()
	}
	return in.UTC()
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	tm := in.UTC()
	return &tm
}
