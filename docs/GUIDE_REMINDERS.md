# Reminders Guide

This guide covers the `pkg/reminders` primitives for host-managed reminder workflows.

---

## Overview

`pkg/reminders` is intentionally small and pure:

- It does not fetch domain records.
- It does not send notifications.
- It does not own eligibility rules.

It only evaluates cadence and anti-spam constraints (`Evaluate`) and computes deterministic next due timestamps (`ComputeNextDue`).

Host applications own domain eligibility, for example:

- Is the agreement still pending?
- Is the recipient in the active signing stage?
- Is the recipient already completed/declined?

---

## Core Types

| Type | Purpose |
|------|---------|
| `reminders.Policy` | Cadence configuration (`initial_delay`, `interval`, caps, cooldowns) |
| `reminders.State` | Execution/view/manual-send signals used for evaluation |
| `reminders.Decision` | Output of `Evaluate`: due flag, reason code, and optional `next_due_at` |

Reason codes:

- `due`
- `disabled`
- `max_count_reached`
- `recent_view_grace`
- `manual_resend_cooldown`
- `not_due_yet`

---

## Quick Start

```go
import (
    "time"

    "github.com/goliatone/go-notifications/pkg/reminders"
)

now := time.Now().UTC()
lastSentAt := now.Add(-25 * time.Hour)

policy := reminders.NormalizePolicy(reminders.Policy{
    Enabled:              true,
    InitialDelay:         24 * time.Hour,
    Interval:             24 * time.Hour,
    MaxCount:             5,
    JitterPercent:        15,
    RecentViewGrace:      2 * time.Hour,
    ManualResendCooldown: 4 * time.Hour,
})

state := reminders.State{
    SentCount:  1,
    LastSentAt: &lastSentAt,
    LastViewedAt: nil,
}

decision := reminders.Evaluate(now, policy, state)
if decision.Due {
    next := reminders.ComputeNextDue(now, policy, "tenant|org|agreement|recipient")
    _ = next // persist as next_due_at after successful send
}
```

---

## Deterministic Jitter

`ComputeNextDue` applies stable jitter around `interval` using a deterministic hash key.

- Use a stable key per reminder target, for example: `tenant|org|agreement|recipient`.
- The same key produces the same jitter offset.
- Different recipients naturally spread out send times and reduce stampede effects.

---

## JSON/Map Helpers

When policy/state is stored as generic JSON maps, use:

- `reminders.PolicyFromMap`
- `reminders.PolicyToMap`
- `reminders.StateFromMap`
- `reminders.StateToMap`

```go
rawPolicy := map[string]any{
    "enabled": true,
    "initial_delay": "24h",
    "interval": "24h",
    "max_count": 5,
    "jitter_percent": 15,
    "recent_view_grace": "2h",
    "manual_resend_cooldown": "4h",
}

policy, err := reminders.PolicyFromMap(rawPolicy)
if err != nil {
    return err
}
```

Duration map fields accept either duration strings (for example `24h`) or numeric seconds.

---

## Recommended Sweep Pattern

1. Claim due candidates with lease/lock semantics in your store.
2. Revalidate domain eligibility in host service.
3. Build `reminders.State` from reminder row + domain signals.
4. Call `Evaluate(now, policy, state)`.
5. If not due, persist skip reason and `next_due_at` from `Decision`.
6. If due, send reminder, increment counters, set `next_due_at = ComputeNextDue(...)`.
7. Release lease.

This separation keeps domain policy in the host while reusing shared cadence logic.

---

## Related Guides

- [GUIDE_DEFINITIONS.md](GUIDE_DEFINITIONS.md)
- [GUIDE_EVENTS.md](GUIDE_EVENTS.md)
- [GUIDE_INTEGRATION.md](GUIDE_INTEGRATION.md)
