# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

(Sona is a native desktop app whose UI MyGo draws on the GPU. It has its own design language rather than the platform's, so `web` is the closest value; the design rules live in `docs/design.md`.)

## Users

Desktop listeners on Linux first, then macOS and Windows, who want Apple Music without a web view. They are keyboard users, and often run Sona tiled at half width beside other work.

## Product Purpose

A fast, native Apple Music client. Go owns every Apple request, the UI is drawn on the GPU, and private listening is honoured everywhere. Success means it feels like part of the desktop, starts and scrolls without a stall, and never leaks what the user listens to privately.

## Positioning

Open source (AGPL-3.0-only), with no web view. Widevine is hosted in a sandboxed child process. Add-ons run beside Sona and never see tokens, Apple hosts or private listens.

## Capabilities and Constraints

- Apple's web API is the boundary: no playlist cover upload and no folder writes. Sona keeps such choices locally (`docs/apple-music-reference.md`).
- The CDM rules in AGENTS.md are binding: never bundled, patched or used outside the CDM host child.
- No layout, text shaping or image work per frame (`docs/architecture.md` §8).
- Add-ons:
  - Sona's own three (Last.fm, ListenBrainz, Discord) are bundled add-ons. Their accounts and switches stay in Settings → Connections.
  - Third-party add-ons are installed by folder, run in an OS sandbox or as WebAssembly, and are approved permission by permission.

## Brand Commitments

"The label's house style" in `docs/design.md` is authoritative: a neutral grey board, one grotesque (Schibsted Grotesk), colour from the music, and one spot colour that means "playing".

## Evidence on Hand

- Real screenshots: `site/src/assets/hero-*.webp`.
- Golden images per surface: `internal/ui/**/testdata/golden`.
- No testimonials, user counts or benchmarks beyond `docs/performance.md`. Don't invent any.

## Product Principles

1. Fast before pretty.
2. Honest states: say plainly what is missing and why, where it would be.
3. Native to the desktop: keyboard first, dense enough to tile.
4. Privacy is structural, not a setting: private listens never leave the window.
