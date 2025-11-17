# Template Authoring & Localization Guide

This guide explains how the Phase 2 template stack works (see `internal/templates`
and `pkg/templates`) and how to author localized templates that play nicely with
go-cms/go-admin adapters.

## Rendering Pipeline Overview

1. `pkg/templates.Service` exposes CRUD helpers plus a `Render` method. It wires
   go-template (Pongo2 based) via the `internal/templates.Service` and injects a
   go-i18n translator so templates can call `t(...)` directly.
2. Templates live in `notification_templates` and may be created via the facade
   (`TemplateInput`) or by optional adapters (e.g., go-cms pumps `TemplateSource`
   payloads into the same repository).
3. At render time the service:
   - Resolves channel/locale variants using the i18n fallback resolver
   - Validates payloads against the stored `TemplateSchema`
   - Injects the resolved locale at `{{ locale }}` so helpers can infer the
     requested language
   - Renders subject/body from inline strings or from go-cms block payloads
   - Caches variants using the configured cache provider + TTL

## Authoring Templates

- **Syntax**: go-template wraps [Pongo2](https://github.com/flosch/pongo2), so
  use Django-style expressions:
  ```django
  {{ Name }}, {{ account.plan }}, {{ t(locale, "welcome.subject", Name) }}
  ```
  Avoid Go template idioms such as `{{ .Name }}` or pipelines.

- **Translation helper**: the translator exposes `{{ t(locale, key, args...) }}`.
  The first argument must be the locale (the service injects `locale` into the
  context). Example:
  ```django
  {{ t(locale, "billing.upcoming", Name, NextInvoiceDate) }}
  ```

- **Schema validation**: capture required placeholders in `TemplateSchema` when
  creating/updating templates. Each field uses dot-notation:
  ```go
  TemplateSchema{
      Required: []string{"Name", "invoice.total"},
      Optional: []string{"account.plan"},
  }
  ```
  Missing `Required` keys produce `SchemaError` before invoking go-template,
  preventing half-rendered notifications.

- **Locale fallback**: if no template exists for the requested locale, the
  service walks the configured fallback chain (e.g., `es-MX → es → en`) before
  returning `ErrNotFound`. No extra logic is required inside templates.

- **Caching**: `pkg/templates.Service` caches each variant using the configured
  cache provider and TTL (`config.Templates.CacheTTL`). Updates automatically
  invalidate the cached entry because the facade writes the new revision back.

## Referencing go-cms / External Sources

The `TemplateSource` field stores structured payloads for adapters. To reference
go-cms blocks set `Source.Type = "gocms-block"` and embed the subject/body:

```go
TemplateInput{
    Code:    "cms.block",
    Channel: "email",
    Locale:  "en",
    Source: domain.TemplateSource{
        Type: "gocms-block",
        Payload: domain.JSONMap{
            "subject": "CMS Subject for {{ Title }}",
            "blocks": []any{
                map[string]any{
                    "body": `<p>{{ t(locale, "welcome.body", Name) }}</p>`,
                },
            },
        },
    },
}
```

The renderer automatically pulls `subject`/`body` fields (or nested `blocks`)
from the payload, so go-cms/go-admin adapters only need to supply the JSON
contract. Inline subject/body strings remain supported for manual templates.

## Using the Facade

```go
translator, _ := i18n.NewSimpleTranslator(store, i18n.WithTranslatorDefaultLocale("en"))
resolver := i18n.NewStaticFallbackResolver()
resolver.Set("es-mx", "es", "en")

svc, _ := templates.New(templates.Dependencies{
    Repository: repo.Templates,
    Cache:      cacheProvider,
    Logger:     &logger.Nop{},
    Translator: translator,
    Fallbacks:  resolver,
    CacheTTL:   time.Minute,
})

tpl, _ := svc.Create(ctx, templates.TemplateInput{
    Code:    "billing.upcoming",
    Channel: "email",
    Locale:  "en",
    Subject: `{{ t(locale, "billing.subject", Name) }}`,
    Body:    `<p>{{ t(locale, "billing.body", Name, Amount) }}</p>`,
    Schema:  domain.TemplateSchema{Required: []string{"Name", "Amount"}},
})

rendered, _ := svc.Render(ctx, templates.RenderRequest{
    Code:    tpl.Code,
    Channel: tpl.Channel,
    Locale:  "es-mx",
    Data: map[string]any{
        "Name":   "Rosa",
        "Amount": "$29.00",
    },
})
```

Adapters such as `adapters/gocms` or `adapters/goadmin` should generate the same
`TemplateInput` records (or `TemplateSource` payloads) so every transport uses
identical rendering semantics.

### Automating Imports with `adapters/gocms`

`adapters/gocms` exposes helpers that convert go-cms block/widget snapshots into
`templates.TemplateInput` values. This keeps the notification module decoupled
from go-cms while still letting operators author templates inside the CMS.

```go
import (
    "context"

    "github.com/goliatone/go-notifications/adapters/gocms"
    "github.com/goliatone/go-notifications/pkg/domain"
    "github.com/goliatone/go-notifications/pkg/templates"
)

func importSnapshot(ctx context.Context, svc *templates.Service, snapshot gocms.BlockVersionSnapshot) error {
    spec := gocms.TemplateSpec{
        Code:        "welcome",
        Channel:     "email",
        Schema:      domain.TemplateSchema{Required: []string{"Name"}},
        Metadata:    domain.JSONMap{"source": "cms"},
    }
    inputs, err := gocms.TemplatesFromBlockSnapshot(spec, snapshot)
    if err != nil {
        return err
    }
    for _, input := range inputs {
        if _, err := svc.Create(ctx, input); err != nil {
            return err
        }
    }
    return nil
}
```

The builder copies go-cms fields (`subject`, `body`, `blocks`, configuration,
metadata) into `TemplateSource.Payload` using the `"gocms-block"` source type.
Locale IDs emitted by go-cms can be mapped to actual locale codes by providing a
`TemplateSpec.ResolveLocale` function. See `docs/NTF_ADAPTERS.md` and the sample
command under `cmd/examples/gocms` for end-to-end usage.
