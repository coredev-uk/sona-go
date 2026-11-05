# Icons

[Lucide](https://lucide.dev) 1.66.0, unmodified SVG files from the npm package
`lucide-static@1.66.0` (`icons/<name>.svg`), the same version the reference
frontend resolved for `lucide-react`. Source: https://github.com/lucide-icons/lucide
and https://www.npmjs.com/package/lucide-static/v/1.66.0.

Licence: ISC, with the Feather MIT notice for the icons derived from Feather;
see [LICENSE](LICENSE) (copied from the package).

## Names

One file per canonical icon. [ALIASES.txt](ALIASES.txt) maps the other names to
the canonical file: `<PascalName> <kebab>` lines for every `lucide-react` name the
reference frontend imported, then `<old-kebab> <kebab>` lines for Lucide's
former names (for example `loader-2 loader-circle`).

## Rule

`icons/` holds exactly the icons the UI uses (the reference frontend's imports plus
the spike's set). To use a new icon, add its `.svg` here and, if you refer to it by
another name, a line to ALIASES.txt. Remove files that are no longer used.

## Refresh

```sh
cd /tmp && curl -fO https://registry.npmjs.org/lucide-static/-/lucide-static-<version>.tgz
tar xzf lucide-static-<version>.tgz
cp package/icons/<name>.svg internal/ui/theme/icons/   # for each name below
cp package/LICENSE internal/ui/theme/icons/LICENSE
```

Canonical names to copy (- app-window - arrow-down - arrow-up-right - arrow-up - badge-check - bell-ring - bell - chart-no-axes-column - check - chevron-down - chevron-left - chevron-right - chevron-up - circle-alert - circle-arrow-up - circle-check - circle-slash - circle-user - circle - clapperboard - clock-3 - clock - command - compass - copy - disc-3 - eighth-notes - ellipsis - external-link - fast-forward - folder-closed - folder-input - folder-open - grip-vertical - hat-glasses - image - infinity - info - key-round - keyboard - languages - link-2 - list-end - list-minus - list-music - list-plus - list-start - loader-circle - lock - map-pin - maximize-2 - maximize - message-square-text - mic-vocal - minimize-2 - minimize - minus - monitor-smartphone - moon - octagon-x - panel-left - pause - pencil - phone - picture-in-picture-2 - pin - play - plus - radio - refresh-cw - repeat-1 - repeat - rewind - rotate-ccw-clock - rotate-ccw - rotate-cw - search - settings - shield-alert - shuffle - skip-back - skip-forward - sliders-vertical - sparkles - spell-check - square-terminal - square - star - text-cursor-input - thumbs-down - ticket - timer-reset - trash - triangle-alert - upload - user - volume-1 - volume-2 - volume-x - wifi-off - x)
