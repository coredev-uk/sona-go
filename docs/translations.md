# Translations reference

The full rules for strings: the UI catalogue, Apple's translations, the pseudo-locale and Go's own text. The [translations page on the site](https://sona.coredev.uk/contributing/translations/) is the short version.

## Strings and translations

Sona takes Apple Music's own translations for every message that means
the same as one of Apple's web strings (docs/ui-architecture.md,
"Strings and locales"). `internal/ui/i18n/locales/apple-keys.json` maps
those messages (English msgid and context) to Apple's string keys, with
placeholder renames; `apple-keys-rejected.json` lists reviewed matches that
do not hold in Sona's context, with the reason. Neither file holds Apple's
text, and Apple's text is never committed or bundled: the app fetches
Apple's bundles from music.apple.com at run time and caches them
(`internal/i18n/applestrings`), and `cmd/i18ntool` fetches them on every
run. The mapping is built into the app, and
`go test ./internal/i18n/applestrings` fails when it no longer fits `en.po`
(a mapped message changed or removed: remove its entry from
`apple-keys.json`, or bring the message back, then re-run `match`).

## Adding or changing a string

The UI's text is written where it is shown, with a string literal:

```go
i18n.T("Add to library")
i18n.T("{count, plural, one {# song} other {# songs}}", "count", n)
i18n.TC("keyboard key", "Home") // context: the same word, another meaning
```

The English text is the message's identity (its msgid), so the message and
the context must be literals (or literals joined with `+`): a variable or a
`fmt.Sprintf` result cannot reach the catalogue, and `extract` rejects it.
Put the whole sentence in one message, with `{name}` arguments and ICU
plurals, never a sentence built from pieces: translators cannot reorder
words they never see together. Changing an English message's wording makes
it a new message, and drops its Apple mapping and its translations.

After adding, changing or removing a call, from the repository root:

```sh
go run ./cmd/i18ntool extract          # writes en.po and en-XA.po
go run ./cmd/i18ntool extract -check   # fails, with the changes, when they are stale
```

`extract` reads every `i18n.T` and `i18n.TC` call under `internal/ui` (test
files and `testdata` excepted) and writes `internal/ui/i18n/locales/en.po`
and `en-XA.po` whole: one entry for each message the code asks for, sorted,
with the Go files that ask (`#: internal/ui/features/…/page.go`) and the
translators' notes (below); a message no call asks for is dropped. The
files are generated, so never edit them. The devenv `check` script and
`go test ./cmd/i18ntool` run the check. It also fails when
`apple-keys.json` maps a message the UI no longer asks for: remove its
entry (or bring the message back). v0.2.0's messages keep their text
byte for byte, so Apple's mapping and any translation of them go on
working; `go test ./internal/i18n/applestrings` pins the 147 mappings
v0.2.0 had.

**Translators' notes.** A comment that starts with `translators:`, directly
above the call or ending on the line the call ends on, becomes the message's
`#.` note in the catalogue, which translators read:

```go
// translators: Equaliser preset
return i18n.T("Flat")

// translators: {0} is the number of songs.
// translators: Said by screen readers when a song starts.
label := i18n.T("{0, plural, one {# song} other {# songs}}", 0, n)
```

Each line starting with `translators:` is one note (the lines after it, up
to the next marker, continue it). Say what the message means where it is
not clear from the text: a word that is a preset, a name or an environment
variable to keep as is, who reads it (screen readers) and what each
argument is. A message asked for in several places gets the notes of all of
them, sorted. Other comments are not notes.

The pseudo-locale finds text that skipped `i18n.T`, is cut or is built from
pieces: `en-XA` shows each message accented, longer and in brackets
(`[Ĥöɱé one]`). `go test ./internal/ui/app` builds the main pages in it and
fails when a message of the catalogue is shown in plain English.

Before writing new English copy, check Apple's wording (its en-GB strings,
keys like `ASE.Web.Music.*`) and use it where the meaning matches. After
`go run ./cmd/i18ntool extract`, from the repository root:

```sh
go run ./cmd/i18ntool match     # counts; proposals for unmapped messages
```

The proposals land in `internal/ui/i18n/locales/apple/proposals.json`
(gitignored; it shows Apple's en-GB text next to each message). Review each
one in context (where the message is shown, `#:` in `en.po`): set
`"decision"` to `"accept"`, or to `"reject"` with a `"reason"`, change
`"key"` to one of `"alternatives"` when another of Apple's keys fits
better, then:

```sh
go run ./cmd/i18ntool match -apply internal/ui/i18n/locales/apple/proposals.json
```

This copies only the message, key and placeholders into the mapping (or the
pair into the rejected list) and checks them against `en.po`. Reject a match
when the meaning differs (a keyboard key against a page name, browser
navigation against skipping a track), when Apple's text is styled for
another place (upper-case badges, lower case only fits mid-sentence), or
when Apple's text drops something Sona's says. `match` also fails when a mapped key has gone from
Apple's en-GB bundle and notes mappings whose English no longer matches.

Wording and case. Sona's English is sentence case (docs/design.md) and
stays so: `match` ignores case, curly quotes, ellipses and closing
punctuation, so "Add to library" maps to Apple's "Add to Library" and a
title-case Apple string is no reason to reject a match. Adopt Apple's
wording, in sentence case, whenever Apple's string names the same action,
concept or state in the same kind of UI place, even when the words differ
(verb, articles, "Back" for "Go back", "Delete from library" for "Remove
from library", no closing ellipsis on a dialog opener, "Top 25 songs by
city"). Keep Sona's own wording only when the meaning differs, Apple's text
would mislead in Sona's context (a keyboard Home against the Home page, the
iPod wheel's Forward against history navigation, Apple's upper-case
badges) or the placeholders cannot map (Apple's "1 Playlist" fixes the
count in its singular form). Record each such refusal in
`apple-keys-rejected.json` with its reason. A message and its siblings
should use one verb: when a button says "Delete from library", its
confirmation, toast and failure message say "delete" too. Most of Sona's
messages still have no Apple counterpart (Apple's en-GB bundle is about
1,000 strings, Sona has about 1,700 of its own), so coverage stays far below
the 90% a locale needs; check Apple's wording before writing a new message,
not afterwards.

```sh
go run ./cmd/i18ntool import    # what the app would get, every locale
```

`import` finds each locale's bundle from music.apple.com's current index
script and runs the app's conversion and checks: `@@name@@` placeholders
become Sona's `{name}`, Apple's `.one`/`.other`/… keys ICU plurals, and a
message whose placeholders do not agree with Sona's is refused. It writes
`internal/ui/i18n/locales/apple/<locale>.po` (gitignored, each entry tagged
`#. apple-key: …`) for review in context, with a per-locale summary (mapped
keys found, missing or refused, coverage of Sona's messages) and each
refusal's reason. It fails when a mapped key has gone from en-GB. The app
never reads these files.

To see Apple's strings in the app while a locale is below the 90% it needs
(`selectableShare` in `internal/ui/i18n`), lower that
value in a local build only; the cache is `$XDG_CACHE_HOME/sona/strings`.

## Go's own text

Text Go shows itself, outside the UI packages (the tray menu, the mini
player's title, desktop notification buttons, file chooser titles and
filters, the sleep inhibitor's reason, and the command line's help, manual
page, completions, output and errors), lives in Go's catalogue,
`internal/i18n/l10n`, never in the UI's. Services never send text to the
UI: they send codes the UI words (`internal/services/errors.go`).
Mark a message by its type, `l10n.Message`, and show it through the user's
`*l10n.Printer` (`Deps.Language.Printer()` in services,
`Options.Language` in the app shell, `Env.Printer` in the command line):

```go
const msgQuit l10n.Message = "Quit Sona" // a translator note

label := p.T(msgQuit)
line := p.Sprintf(msgSearchNone, term) // msgSearchNone = "no results for %q"
```

A typed constant or variable, a conversion `l10n.Message("…")` or the
values of a `map[K]l10n.Message` or `[]l10n.Message` literal are messages;
their value must be a string literal (or literals joined with `+`), or the
name of another message. The declaration's comment becomes the
translator's note. Messages are `fmt` formats: a translation keeps the
English verbs and may reorder them (`%[2]s`), and the app ignores one
whose verbs differ (extraction flags such messages `go-format` so
translators and their tools know). Text that is the same in every language
stays a plain string: command and option names, usage lines, file names,
protocol strings (MPRIS errors), logs and the diagnostics bundle.

After adding or changing a message, from the repository root:

```sh
go run ./cmd/i18ntool extract          # writes internal/i18n/l10n/locales/en.po
go run ./cmd/i18ntool extract -check   # fails, with the changes, when it is stale
```

`en.po` is generated (sorted, with the files each message comes from and
an empty `msgstr`); `go test ./internal/i18n/l10n/extract` and the devenv
`check` script fail when it is out of date. A translation is
`internal/i18n/l10n/locales/<tag>.po`: the same msgids with `msgstr`
filled in (fuzzy entries are skipped), embedded in the binary and picked
for the Language setting, else the system's languages, as the UI
picks its catalogue.

