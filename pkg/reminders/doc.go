// Package reminders provides reusable reminder cadence primitives.
//
// Host applications own domain eligibility (for example agreement status,
// recipient stage, or lifecycle constraints). This package only evaluates
// cadence/no-spam constraints and computes deterministic next-due timestamps.
package reminders
