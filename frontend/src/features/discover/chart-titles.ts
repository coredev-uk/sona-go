import type { Chart } from "@bindings/services";

/** Chart names in Sona's words, by chart key. */
export const chartTitles: Record<string, string> = {
  songs: "Top songs",
  albums: "Top albums",
  playlists: "Top playlists",
  "music-videos": "Top music videos",
  cityCharts: "Top 25 by city",
  dailyGlobalTopCharts: "Daily top 100 by country",
};

export function chartTitle(c: Pick<Chart, "key" | "title">) {
  return chartTitles[c.key] ?? c.title;
}
