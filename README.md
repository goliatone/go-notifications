# go-notifications

Composable notification services for Go applications. This module follows the technical design defined in `docs/NTF_TDD.md` and ships:

- Multi-channel dispatch powered by pluggable adapters
- Template rendering + localization via go-template and go-i18n
- Preferences, inbox state, and options layering driven by go-options
- Adapter-oriented integrations so go-admin/go-cms remain optional

## Status

Architecture and implementation plans live in `docs/NTF_TDD.md` and `docs/NTF_TSK.md`. Phase 0 tasks bootstrap the repository layout, tooling, and configuration skeleton before services are implemented.

## Development

Use the Taskfile targets once the Task CLI is installed:

```bash
task lint
task test
task docs:lint
task ci
```

See `CONTRIBUTING.md` for contribution guidelines.
