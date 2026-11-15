---
title: Add a Feature
description: Add a page or feature as a Go service plus a UI package.
---

A feature is a Go service and a UI package, each self-contained. Features never call each other's services or import each other's packages.

## Add the service

1. Create `internal/services/<feature>.go`. The UI calls its exported methods.
2. Create `internal/services/<feature>_dto.go` for its DTOs and events. DTOs are UI-shaped: durations in milliseconds, artwork as `/art/...` cache paths.
3. Add one line to `internal/services/registry.go`.

The full contract is in `internal/services/doc.go`.

## Add the UI package

1. Create `internal/ui/features/<name>`. Its factory takes a `*module.Env` and returns a `module.Module`: its routes, sidebar entries, shortcuts and event handlers. Keep state in one struct and make views methods on it.
2. Add one line to `internal/ui/app/modules.go`. Its order decides the order of routes, handlers, shortcuts and slots.

```go
func New(env *module.Env) module.Module {
    l := &library{env: env}
    return module.Module{
        ID:     "library",
        Routes: []module.Route{{Pattern: "/library/albums", Title: func(*ui.Route) string { return i18n.T("Albums") }, Build: l.albums}},
        Nav:    []module.NavItem{{Section: module.NavLibrary, Label: func() string { return i18n.T("Albums") }, Icon: "disc-3", To: "/library/albums", Order: 1}},
        Init:   l.init,
    }
}
```

Shortcut IDs (`<module>.<action>`) are stored in users' settings, so never rename one. See [UI Conventions](/contributing/ui-conventions/) and the full contract in [docs/ui-architecture.md](https://github.com/coredev-uk/sona-go/blob/main/docs/ui-architecture.md).

## Add settings

Add one nested struct to `internal/config/settings.go` with its defaults and `normalized()`. Apply changes live through `Settings.OnChange`.

## Finish

1. Design the page fresh; Apple's apps are a functional reference, never a layout to copy.
2. Write strings with `i18n.T` (see [Translations](/contributing/translations/)).
3. Add tests with `ui.Tester` and run `check` and `tests` ([Test Your Change](/contributing/test/)).
4. Update the docs in the same pull request.
