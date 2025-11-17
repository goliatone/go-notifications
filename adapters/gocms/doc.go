package gocms

// Package gocms provides helpers that translate go-cms block/widget payloads
// into go-notifications template inputs without importing go-cms directly.
// Callers decode the JSON snapshots emitted by go-cms (blocks, widgets, etc.)
// into the lightweight structs here and then convert them into
// templates.TemplateInput records via TemplatesFromBlockSnapshot or
// TemplatesFromWidgetDocument.
