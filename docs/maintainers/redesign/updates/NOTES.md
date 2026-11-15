# Update notice: "Release slip"

**Direction.** The sidebar footer notice becomes a compact slip, a quiet block rather than a line with a text link.

**Composition.** A `raised` block (radius 6, padding 10, inset 10 DIP from the sidebar edges, 8 below): "New version" in meta `ink-muted`, "Sona {latest}" at 14/500, then a full-width small ink-filled `kit.Button` ("Install and restart" or "View"). Installing: the line reads "Downloading Sona {latest}…", the button gives way to the progress bar inside the slip. Installed: "Restarting into Sona {latest}…", no action. Failed install: "Couldn't install Sona {latest}" with the button still there.

**Accessibility.** The status element is "Sona {latest}" but carries the label "Sona {latest} is available" and announces that on mount, so the announcement and status name are unchanged.

**Tests.** `updates_test.go`: visible-text assertions moved from "Sona 0.3.0 is available" to "New version" / "Sona 0.3.0" (the sentence was split across two lines). a11y tests unchanged and passing. Added `golden_test.go` (`slip-light/dark`, 232×120).

**New strings.** "New version", "Sona {latest}".
