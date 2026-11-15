---
title: Translations
description: Write strings for translation and keep the catalogues in step.
---

Every string the interface shows goes through `i18n.T`. The English text is the message's identity, and the catalogues are generated from the code.

## Add or change a string

1. Write the string where it is shown:

   ```go
   i18n.T("Add to library")
   i18n.T("{count, plural, one {# song} other {# songs}}", "count", n)
   i18n.TC("keyboard key", "Home") // context: the same word, another meaning
   ```

2. Regenerate the catalogue from the repository root:

   ```sh
   devenv shell -- go run ./cmd/i18ntool extract
   ```

3. Commit the changed `.po` files. Never edit them by hand.

Follow these rules:

- The message and its context must be string literals (or literals joined with `+`). A variable or `fmt.Sprintf` result can't reach the catalogue.
- Put the whole sentence in one message, with `{name}` arguments and ICU plurals. Never build a sentence from pieces: translators can't reorder words they never see together.
- Changing an English message's wording makes it a new message and drops its translations.
- Write titles (page and section headings, nav items, dialog titles, tab, settings group and column names) in Title Case, Apple's style: "Made for You", "Songs in Your Library". Write buttons, menu items, body text, hints and errors in sentence case: "Add to library".

`extract -check` fails, with the changes, when the catalogue is stale. `check` and `go test ./cmd/i18ntool` run it.

## Add a note for translators

A comment starting with `translators:` directly above the call becomes a note in the catalogue:

```go
// translators: Equaliser preset
return i18n.T("Flat")
```

Say what the message means where the text doesn't: a preset, a name to keep as is, who reads it (screen readers) and what each argument is.

## Check the pseudo-locale

`en-XA` shows each message accented, longer and in brackets (`[Ĥöɱé one]`). `go test ./internal/ui/app` builds the main pages in it and fails when a message appears in plain English, which finds text that skipped `i18n.T`, is cut or is built from pieces.

## Use Apple's wording

Sona adopts Apple Music's own translations for messages that mean the same as one of Apple's web strings. Check Apple's wording before writing new copy. Run `go run ./cmd/i18ntool match` for proposals, review each in context, then apply them. Apple's text is never committed or bundled.

## Text Go shows itself

Text outside the UI packages (the tray menu, notification buttons, file chooser titles, the command line's help and errors) lives in Go's own catalogue, `internal/i18n/l10n`. Mark a message with the `l10n.Message` type and show it through the user's `*l10n.Printer`. Services never send text to the UI: they send codes the UI words. Run the same `extract` command to update `internal/i18n/l10n/locales/en.po`.

## Go further

[docs/translations.md](https://github.com/coredev-uk/sona-go/blob/main/docs/translations.md) has the full rules: Apple's mapping and its review workflow, wording and case, and Go's catalogue.
