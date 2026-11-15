---
title: UI Conventions
description: The UI's modules, kit, design tokens, performance rules and accessibility checks.
---

Sona's interface is Go all the way down: no web view and no IPC. A view calls a typed service and reads the result. Full references: [docs/ui-architecture.md](https://github.com/coredev-uk/sona-go/blob/main/docs/ui-architecture.md), [docs/design.md](https://github.com/coredev-uk/sona-go/blob/main/docs/design.md) and [docs/accessibility.md](https://github.com/coredev-uk/sona-go/blob/main/docs/accessibility.md).

## Modules

| Package | Holds |
| --- | --- |
| `internal/ui/app` | The windows, sidebar, router and shortcut dispatcher. `modules.go` lists the features. |
| `internal/ui/module` | The contract between the shell and a feature. Types only. |
| `internal/ui/features/<name>` | One package per feature. Features never import each other. |
| `internal/ui/store` | The event bus, cached queries and the playback mirror. |
| `internal/ui/theme` | Design tokens, icons, fonts. |
| `internal/ui/kit` | Shared components. |
| `internal/ui/i18n` | `T`, `TC` and the locale. |

A feature reaches another's state only through `store` or a hook on `module.Env`. To add one, see [Add a Feature](/contributing/add-a-feature/).

- Views are methods on the feature's state struct. Never store a build value (`ui.Element`); `go tool mygo vet internal/ui` (part of `check`) finds one kept past its frame.
- Service calls, decoding and file I/O run off the main thread. Results come back through `store` or the Poster.
- Logs never carry tokens or personal data.

## Kit

Components are functions that take the `*ui.Context`: `kit.Button(c, label, kit.ButtonOpts{...})`. Give repeated elements a stable key with `c.Key(id)`. Reuse a kit component before building a new one. Browse them all in the [gallery](/contributing/build-and-run/#browse-the-component-gallery), and add a gallery section with any new kit component.

## Tokens

`internal/ui/theme/tokens.json` is the one source of colours, type, radii, space and motion, light and dark. Read them by name and never hard-code a colour or a size:

```go
theme.C("ink")
theme.Current().Radius["md"]
```

Call `theme.Apply(c)` at the top of every view. A development build reloads the tokens file without a rebuild. Themes users write are described in [Theme File Format](/reference/theme-format/).

## Design rules

- Fast before pretty: nothing visual may cost first paint or scroll smoothness.
- The cover is the colour. Chrome stays neutral so artwork reads true.
- The spot colour (amber in Sona's own theme) means "playing" and nothing else.
- Light and dark follow the system. Everything works from the keyboard.
- State plainly what is missing: signed out, syncing, offline, no CDM.
- Design every page fresh. Apple's apps are a functional reference, never a layout to copy.

## Performance rules

- Do no layout, text shaping or image work per frame. Invalidate narrowly.
- Use `Scroll` (or your own virtualisation) for long lists.
- Decode artwork at the size it is shown, off the main thread, and cache it.
- Pre-blur backdrops in Go. Never blur live.
- Respect Reduce effects and reduced motion (`theme.ReduceEffects()`, `theme.ReducedMotion()`).

To measure a change, see [docs/performance.md](https://github.com/coredev-uk/sona-go/blob/main/docs/performance.md).

## Accessibility

Sona targets WCAG 2.2 AA in both themes. Every control needs a role and a name, and a control is never nested inside a button or link, because assistive technology reads those as one piece. Text tokens keep at least 4.5:1 contrast.

The checks run in `tests` and `e2e`:

- `uitest/access` reads a `ui.Tester` window's accessibility tree and walks Tab round it. It fails a nested control or a keyboard trap.
- Per-feature `a11y_test.go` files check each page's tree and focus order.
- `TestContrastFloors` checks the text tokens' contrast.
- `e2e` reads the real app's tree over AT-SPI.

A person with a screen reader still checks what these can't. The rules and the `orca-walk` script are in [docs/accessibility.md](https://github.com/coredev-uk/sona-go/blob/main/docs/accessibility.md).
