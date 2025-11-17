# Optional Adapter Guide

Phase 7 introduces optional adapters that keep go-admin/go-cms dependencies
outside of the core module. This document describes the go-cms integration
helpers shipped in `adapters/gocms` and how host applications can reuse them.

## go-cms Snapshot Adapter

The go-cms runtime stores block/widget content as JSON snapshots. The adapter
package converts those snapshots into `templates.TemplateInput` records so the
notification module can persist variants without importing go-cms.

### Snapshot Format

The adapter mirrors the JSON emitted by go-cms:

```json
{
  "configuration": {"layout": "hero"},
  "metadata": {"definition": "notification.hero"},
  "translations": [
    {
      "locale": "locale-en",
      "content": {
        "subject": "Welcome {{ Name }}",
        "body": "<p>Hello {{ Name }}</p>",
        "blocks": [
          {"type": "richtext", "body": "<p>{{ Name }}</p>"}
        ]
      },
      "attribute_overrides": {
        "body": "<p>Hola {{ Name }}</p>"
      }
    }
  ]
}
```

Callers decode the JSON into `gocms.BlockVersionSnapshot` (or
`gocms.WidgetDocument` for widget instances) and pass it to the conversion
helpers.

### TemplateSpec Settings

`gocms.TemplateSpec` captures the notification-specific metadata applied to every
variant. It enforces the template code/channel and lets hosts customize field
mappings and locale resolution:

```go
spec := gocms.TemplateSpec{
    Code:        "welcome",
    Channel:     "email",
    Description: "Auth welcome notifications",
    Schema:      domain.TemplateSchema{Required: []string{"Name"}},
    Metadata:    domain.JSONMap{"source": "cms"},
    Fields: gocms.FieldMapping{Blocks: "sections"}, // optional overrides
    ResolveLocale: func(raw string) (string, error) {
        // go-cms stores locale IDs, so look up the code you want to persist.
        return localeProvider.ResolveCode(raw)
    },
}
```

### Block Snapshot Conversion

```go
snapshot := gocms.BlockVersionSnapshot{
    Configuration: cfg,
    Metadata:      meta,
    Translations:  translations,
}

inputs, err := gocms.TemplatesFromBlockSnapshot(spec, snapshot)
if err != nil {
    return err
}
for _, input := range inputs {
    _, err := templateService.Create(ctx, input)
    if err != nil {
        return err
    }
}
```

The adapter copies subject/body/preheader/blocks into `TemplateSource.Payload`
using the `TemplateSourceType` constant (`"gocms-block"`). The metadata and
configuration maps are cloned so later mutations in go-cms do not impact stored
templates.

### Widget Document Conversion

Widget instances reuse the same API via `TemplatesFromWidgetDocument`. The only
difference is the input struct (`gocms.WidgetDocument` + `gocms.WidgetTranslation`)
which mirrors the widget translation JSON.

```go
doc := gocms.WidgetDocument{
    Configuration: widgetConfig,
    Metadata:      placementMeta,
    Translations:  widgetTranslations,
}
inputs, err := gocms.TemplatesFromWidgetDocument(spec, doc)
```

### Sample Command

`cmd/examples/gocms` demonstrates the full flow with synthetic data and logs the
render-ready payload. The Taskfile exposes this via `task samples` so CI
continuously verifies the example builds alongside unit/integration tests.
