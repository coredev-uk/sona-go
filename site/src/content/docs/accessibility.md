---
title: Accessibility
description: What works with the keyboard, screen readers and reduced motion, and the known gaps.
---

Sona aims for WCAG 2.2 AA in every built-in theme. This page says what works and what does not.

## What works

- **Keyboard**: every control is reachable. Lists are one Tab stop with the arrow keys inside. Menus open with Shift+F10 or the Menu key. Every drag has a keyboard alternative, such as Alt+↑ and Alt+↓ in the queue. See [Keyboard Shortcuts](/guide/keyboard-shortcuts/).
- **Screen readers**: Sona talks to Orca on Linux, VoiceOver on macOS, and Narrator and NVDA on Windows. Controls have names, and track changes are announced.
- **Contrast**: text meets 4.5:1 in every theme. The High contrast [theme](/guide/themes/) reaches 7:1.
- **Reduced motion**: Sona follows your system's reduce-motion setting. Transitions stop, lyrics light each word whole, and motion artwork shows its still.

## Known gaps

- Only Orca has been tested with Sona. VoiceOver, Narrator and NVDA are untested.
- When Now Playing opens from a key, Escape leaves the focus on nothing. Press Tab to continue from the sidebar.
- Screen readers hear a page's generic title on arrival ("Album"), not the album's name.
- Some shortcuts that open a panel, such as Q for the queue, say nothing.
- On Linux and macOS, `?` needs the focus in the window. Press Tab first.

## Report a problem

Open an issue on [GitHub](https://github.com/coredev-uk/sona-go/issues/new?template=bug_report.yml) and say which screen reader and system you use. The technical rules are in [docs/accessibility.md](https://github.com/coredev-uk/sona-go/blob/main/docs/accessibility.md).
