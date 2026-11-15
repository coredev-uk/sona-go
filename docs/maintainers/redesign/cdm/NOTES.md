# Widevine notice: "Inline progress"

**Direction.** The line under the title shows the download instead of describing it: the percent sits beside a small meter.

**Composition.** Downloading: the link "Waiting for Widevine to download", a 48×3 DIP `kit.ProgressBar` (label "Widevine download" / "Widevine update", an empty track while the size is unknown) and the percent in `ink-muted`; the percent used to be inside the sentence. Failed: a 6 DIP `danger` dot (decorative; the words say it) before the Retry-in-Settings link, or "Songs can't play on this computer". Ready: a check before "Widevine is ready. Press play to start." The link to Settings → Playback keeps its route rule.

**States.** Waiting (no status), downloading with and without a total, idle, failed (network, unsupported platform), ready; with and without the `/settings` route.

**Tests.** `TestNoticeWordsEachState` now expects the sentence and "42%" as two texts (the percent left the sentence). Added `golden_test.go` (`notice-downloading-light/dark`, 360×48).

**Constraints.** Reuses the msgids "Widevine download" and "Widevine update" from Settings. The old msgid "Waiting for Widevine to download · {percent}" is no longer used (catalogues are regenerated centrally).
