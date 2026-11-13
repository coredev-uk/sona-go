# Account dialog and account menu: Two-panel sign-in

## Direction

The old dialog was a form with a title on top: "Account", then the form's own "Sign in to Apple Music", then the fields in 512px of one column. The new one separates *why* from *how*. A leading panel on `--surface` carries the Sona mark, one heading and three single lines on what signing in adds; the account's panel beside it holds only the task. The dialog is wider (680) and the form narrower (327 inside its inset), so the fields read as a form, not a banner. The menu keeps its rows and order and gains legibility: every fact about the account is a glyph-led line, and the avatar reads as held while its menu is open.

## Composition by region

- **Dialog frame.** `kit.Dialog` in `Bare` mode with the title hidden (it still names the dialog for assistive technology), 680 wide. The panels sit in one `Scroll` capped to the window less its 2 rem margins, so in a short window the lead's tone scrolls with the form.
- **Lead (280, `surface`).** The mark (the sidebar's record label, 22px) and "Sona" on top; the heading "Your Apple Music, on your desktop" (24/30, h2) and three lines (library, recommendations, whole songs) at the bottom, each behind a 16px glyph. 36px inset all round. A 1px `line` divides it from the account.
- **Account panel.** `account.Section.Build(c, false)` unchanged, 36px inset; its first heading and the lead's mark row sit on one centre line (both 22 high at the same inset). Signed in, the panel adds the "Account" heading (h3) the section does not bring, since the dialog's own visible title is gone.
- **Narrow (panel < 640).** Stacked: lead band (mark, heading, lines) above, a hairline, then the account. The window is never narrower than 940, so this is for a shell that lets the window shrink; it is tested at 600.
- **Menu.** Header "Apple Music", then the state line, offline, restricted and unencrypted notes each with a 14px glyph (check, clock, alert in `danger`, wifi-off, lock, shield-alert in `danger`). Items, order, checkbox and danger Sign out unchanged. The avatar takes the `active` tone and ink glyph while the menu is open.

## States covered

Signed out (form, light and dark), signed in, the reauth notice, loading and restoring placeholders and the error block (all inside the account's panel, unchanged), stacked narrow, and the menu signed out, signed in, offline, restricted, unencrypted, keyring trouble.

## Constraints hit

- `internal/ui/account` is outside the boundary and titles the sign-in flow itself ("Sign in to Apple Music", h3), so the lead heading is a different line and the dialog's h2 moved from "Account" to the lead heading.
- `kit.Menu` draws header rows as text (a `MenuLabel` with an optional 14px icon) and cannot hold custom content, so the avatar disc in the menu header is not possible without changing the kit. The header carries state glyphs instead. The theme has no log-out or library icons, so the action rows stay unadorned.
- `AuthStatus` has no account name or picture, so nothing personal is shown.

## Tests

- `TestDialogKeyboardWalk`: the h2 check names the lead heading, not "Account" (the dialog is still named "Account"; Tab, Escape, focus return unchanged).
- `TestMenuHeaderAndNotesAreStyled`: also pins that the state line starts after its glyph.
- New: `TestDialogHasATitleSignedInAndStacksWhenNarrow` (signed-in heading; stacked order and Tab walk at 600 wide).
- Goldens: `account-dialog-signed-out-{light,dark}` regenerated; new `account-dialog-signed-in-light`, `account-dialog-stacked-dark`, `account-menu-light`.
- Nothing deleted.

## New strings

"Your Apple Music, on your desktop", "Your library, in sync", "Recommendations for you", "Whole songs, not previews". "Account" and "Sona" are reused.
