// Package reminders provides reusable reminder cadence primitives.
//
// Host applications own domain eligibility (for example agreement status,
// recipient stage, or lifecycle constraints). This package only evaluates
// cadence/no-spam constraints and computes deterministic next-due timestamps.
//
// Typical usage:
//
//  1. Build a Policy (or parse one with PolicyFromMap).
//  2. Build a State from persisted reminder execution signals.
//  3. Call Evaluate(now, policy, state) to decide if due.
//  4. After a successful send, call ComputeNextDue(now, policy, stableKey).
//
// Stable keys should be deterministic per reminder target (for example
// tenant|org|entity|recipient) to spread traffic with deterministic jitter.
package reminders
