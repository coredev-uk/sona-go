---
title: Accessibility
description: What works with the keyboard, screen readers and reduced motion, what has been tested, and the known gaps.
---

Sona aims for WCAG 2.2 AA in every built-in theme. This page says what works, what has been tested and what does not work yet.

## What works

- **Keyboard**: every control is reachable. Lists are one Tab stop with the arrow keys inside. Menus open with Shift+F10 or the Menu key, or with a right-click. Every drag has a keyboard alternative, such as Alt+↑ and Alt+↓ in the queue. See [Keyboard Shortcuts](/guide/keyboard-shortcuts/).
- **Screen readers**: Sona talks to Orca on Linux, VoiceOver on macOS, and Narrator and NVDA on Windows. Controls have names, the state of a control is said (pressed, current page, busy, unavailable), and track changes and errors are announced once.
- **Contrast**: text meets 4.5:1 in every theme. The High contrast [theme](/guide/themes/) reaches 7:1.
- **Reduced motion**: Sona follows your system's reduce-motion setting. Transitions stop, lyrics light each word whole (the dots of an instrumental gap light whole too), and motion artwork shows its still. In Full-Screen Lyrics the lines jump to their place and the background doesn't drift.

## What has been tested

| System | Screen reader | Tested |
| --- | --- | --- |
| Linux | Orca | Walked through the whole app by keyboard in an automated test, reading what Orca says. Not yet heard through speech in a desktop session, or while signed in |
| macOS | VoiceOver | The app's accessibility tree is tested. Nobody has listened to VoiceOver with Sona yet |
| Windows | Narrator, NVDA | Not tested. Nobody has listened to either with Sona yet |

## Known gaps

- When Now Playing opens from a key (N or L), Escape leaves the focus on nothing. Press Tab to continue from the sidebar.
- A confirmation that opens over the first-run setup returns the focus to the setup's first control, not to where you were.
- Screen readers hear a page's generic title on arrival ("Album"), not the album's name.
- Some shortcuts that open a panel, such as Q for the queue, say nothing.
- On Linux and macOS, `?` needs the focus in the window. Press Tab first, or use the command palette (Ctrl+K). On some keyboard layouts the help opens with a different key than the one printed `?`.
- Orca does not announce the Player bar or Now playing areas as the focus enters them.
- A shortcut cannot be bound to F13 to F24.
- A button that opens and closes something, other than a menu, does not say whether it is open.
- Leaving the Seek slider says its time once more.

## Report a problem

Open an issue on [GitHub](https://github.com/coredev-uk/sona-go/issues/new?template=bug_report.yml) and say which screen reader and system you use. The technical rules are in [Accessibility Rules](/contributing/accessibility/).
