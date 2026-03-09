package reminders

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// PolicyFromMap converts a generic map to a Policy.
// Duration fields accept duration strings (for example "24h") or numeric seconds.
func PolicyFromMap(input map[string]any) (Policy, error) {
	if len(input) == 0 {
		return Policy{}, nil
	}
	out := Policy{}
	if value, ok, err := readBool(input, "enabled"); err != nil {
		return Policy{}, err
	} else if ok {
		out.Enabled = value
	}
	if value, ok, err := readDuration(input, "initial_delay"); err != nil {
		return Policy{}, err
	} else if ok {
		out.InitialDelay = value
	}
	if value, ok, err := readDuration(input, "interval"); err != nil {
		return Policy{}, err
	} else if ok {
		out.Interval = value
	}
	if value, ok, err := readInt(input, "max_count"); err != nil {
		return Policy{}, err
	} else if ok {
		out.MaxCount = value
	}
	if value, ok, err := readInt(input, "jitter_percent"); err != nil {
		return Policy{}, err
	} else if ok {
		out.JitterPercent = value
	}
	if value, ok, err := readDuration(input, "recent_view_grace"); err != nil {
		return Policy{}, err
	} else if ok {
		out.RecentViewGrace = value
	}
	if value, ok, err := readDuration(input, "manual_resend_cooldown"); err != nil {
		return Policy{}, err
	} else if ok {
		out.ManualResendCooldown = value
	}
	return out, nil
}

// PolicyToMap converts a Policy to map form using duration strings.
func PolicyToMap(policy Policy) map[string]any {
	return map[string]any{
		"enabled":                policy.Enabled,
		"initial_delay":          policy.InitialDelay.String(),
		"interval":               policy.Interval.String(),
		"max_count":              policy.MaxCount,
		"jitter_percent":         policy.JitterPercent,
		"recent_view_grace":      policy.RecentViewGrace.String(),
		"manual_resend_cooldown": policy.ManualResendCooldown.String(),
	}
}

// StateFromMap converts a generic map to State.
// Timestamp fields accept RFC3339 strings or unix seconds.
func StateFromMap(input map[string]any) (State, error) {
	if len(input) == 0 {
		return State{}, nil
	}
	out := State{}
	if value, ok, err := readInt(input, "sent_count"); err != nil {
		return State{}, err
	} else if ok {
		out.SentCount = value
	}
	if value, ok, err := readTime(input, "first_sent_at"); err != nil {
		return State{}, err
	} else if ok {
		out.FirstSentAt = &value
	}
	if value, ok, err := readTime(input, "last_sent_at"); err != nil {
		return State{}, err
	} else if ok {
		out.LastSentAt = &value
	}
	if value, ok, err := readTime(input, "last_viewed_at"); err != nil {
		return State{}, err
	} else if ok {
		out.LastViewedAt = &value
	}
	if value, ok, err := readTime(input, "last_manual_resend_at"); err != nil {
		return State{}, err
	} else if ok {
		out.LastManualResendAt = &value
	}
	if value, ok, err := readTime(input, "next_due_at"); err != nil {
		return State{}, err
	} else if ok {
		out.NextDueAt = &value
	}
	return out, nil
}

// StateToMap converts State to a generic map with RFC3339 timestamps.
func StateToMap(state State) map[string]any {
	out := map[string]any{
		"sent_count": state.SentCount,
	}
	if state.FirstSentAt != nil {
		out["first_sent_at"] = state.FirstSentAt.UTC().Format(time.RFC3339Nano)
	}
	if state.LastSentAt != nil {
		out["last_sent_at"] = state.LastSentAt.UTC().Format(time.RFC3339Nano)
	}
	if state.LastViewedAt != nil {
		out["last_viewed_at"] = state.LastViewedAt.UTC().Format(time.RFC3339Nano)
	}
	if state.LastManualResendAt != nil {
		out["last_manual_resend_at"] = state.LastManualResendAt.UTC().Format(time.RFC3339Nano)
	}
	if state.NextDueAt != nil {
		out["next_due_at"] = state.NextDueAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func readInt(input map[string]any, key string) (int, bool, error) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	switch typed := raw.(type) {
	case int:
		return typed, true, nil
	case int32:
		return int(typed), true, nil
	case int64:
		return int(typed), true, nil
	case float64:
		return int(typed), true, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, false, nil
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, false, fmt.Errorf("reminders: invalid %s integer", key)
		}
		return parsed, true, nil
	default:
		return 0, false, fmt.Errorf("reminders: invalid %s type", key)
	}
}

func readBool(input map[string]any, key string) (bool, bool, error) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return false, false, nil
	}
	switch typed := raw.(type) {
	case bool:
		return typed, true, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return false, false, nil
		}
		parsed, err := strconv.ParseBool(trimmed)
		if err != nil {
			return false, false, fmt.Errorf("reminders: invalid %s boolean", key)
		}
		return parsed, true, nil
	default:
		return false, false, fmt.Errorf("reminders: invalid %s type", key)
	}
}

func readDuration(input map[string]any, key string) (time.Duration, bool, error) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	switch typed := raw.(type) {
	case time.Duration:
		return typed, true, nil
	case int:
		return time.Duration(typed) * time.Second, true, nil
	case int64:
		return time.Duration(typed) * time.Second, true, nil
	case float64:
		return time.Duration(typed) * time.Second, true, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, false, nil
		}
		if parsed, err := time.ParseDuration(trimmed); err == nil {
			return parsed, true, nil
		}
		seconds, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false, fmt.Errorf("reminders: invalid %s duration", key)
		}
		return time.Duration(seconds) * time.Second, true, nil
	default:
		return 0, false, fmt.Errorf("reminders: invalid %s type", key)
	}
}

func readTime(input map[string]any, key string) (time.Time, bool, error) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return time.Time{}, false, nil
	}
	switch typed := raw.(type) {
	case time.Time:
		return typed.UTC(), true, nil
	case int64:
		return time.Unix(typed, 0).UTC(), true, nil
	case float64:
		return time.Unix(int64(typed), 0).UTC(), true, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return time.Time{}, false, nil
		}
		if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return parsed.UTC(), true, nil
		}
		seconds, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("reminders: invalid %s timestamp", key)
		}
		return time.Unix(seconds, 0).UTC(), true, nil
	default:
		return time.Time{}, false, fmt.Errorf("reminders: invalid %s type", key)
	}
}
