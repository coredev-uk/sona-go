# Release reminder menu entry: "Dated reminder"

**Direction.** A reminder is for a day, so the album menu entry names it: "Remind me on Nov 22" / "Turn off reminder for Nov 22" (`i18n.ShortReleaseDate` of `Album.ReleaseAt`, the year added off this year, following the regional format). Without a date (not listable anyway) the old plain labels remain.

**Unchanged.** Action ID `reminders.toggle`, target, group, order 25, availability, enabled-after-load rule, run.

**Tests.** The label assertions in `reminders_test.go` and `a11y_test.go` are dated now; added `TestTheLabelNamesTheDayAndFallsBackWithoutOne`.

**Constraints.** `kit.ActionDef.Icon` is a static name and the theme icon set has `bell` and `bell-ring` but no `bell-off`, and kit/theme are out of bounds: the entry keeps the `bell` icon in both states, the label carries the on/off meaning.

**New strings.** "Remind me on {date}", "Turn off reminder for {date}".
