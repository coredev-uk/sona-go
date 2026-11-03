import { t } from "@lingui/core/macro";
import type { Chart } from "@bindings/services";

/** Chart names in Sona's words, by chart key; Apple's title for any other chart. */
export function chartTitle(c: Pick<Chart, "key" | "title">): string {
  switch (c.key) {
    case "songs":
      return t`Top songs`;
    case "albums":
      return t`Top albums`;
    case "playlists":
      return t`Top playlists`;
    case "music-videos":
      return t`Top music videos`;
    case "cityCharts":
      return t`Top 25 songs by city`;
    case "dailyGlobalTopCharts":
      return t`Top 100 songs by country`;
    default:
      return c.title;
  }
}
