import type { Concert } from "@bindings/services";
import { i18n } from "@lingui/core";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  distanceLabel,
  groupShows,
  presetRange,
  rangeFor,
  showDate,
  showDay,
  showTime,
  showWeekday,
} from "./dates";

const show = (iso: string, timezone: string, id = iso): Concert =>
  ({ id, startsAt: Date.parse(iso), timezone }) as Concert;

describe("presetRange", () => {
  // Wednesday 4 November 2026.
  const wed = new Date(2026, 10, 4, 15, 0);
  it("is nothing for any time", () => {
    expect(presetRange(undefined, wed)).toBeUndefined();
  });
  it("runs this week to Sunday", () => {
    expect(presetRange("week", wed)).toEqual({ from: "2026-11-04", to: "2026-11-08" });
    expect(presetRange("week", new Date(2026, 10, 8))).toEqual({
      from: "2026-11-08",
      to: "2026-11-08",
    });
  });
  it("is Friday to Sunday, or what is left of the weekend", () => {
    expect(presetRange("weekend", wed)).toEqual({ from: "2026-11-06", to: "2026-11-08" });
    expect(presetRange("weekend", new Date(2026, 10, 7))).toEqual({
      from: "2026-11-07",
      to: "2026-11-08",
    });
    expect(presetRange("weekend", new Date(2026, 10, 8))).toEqual({
      from: "2026-11-08",
      to: "2026-11-08",
    });
    expect(presetRange("weekend", new Date(2026, 10, 6))).toEqual({
      from: "2026-11-06",
      to: "2026-11-08",
    });
  });
  it("is thirty days from today, across a month and a year end", () => {
    expect(presetRange("month", wed)).toEqual({ from: "2026-11-04", to: "2026-12-03" });
    expect(presetRange("month", new Date(2026, 11, 20))).toEqual({
      from: "2026-12-20",
      to: "2027-01-18",
    });
  });
});

describe("rangeFor", () => {
  const now = new Date(2026, 10, 4);
  it("takes typed dates only when both are there and in order", () => {
    expect(rangeFor({ when: "dates", from: "2026-12-01", to: "2026-12-24" }, now)).toEqual({
      from: "2026-12-01",
      to: "2026-12-24",
    });
    expect(rangeFor({ when: "dates", from: "2026-12-01" }, now)).toBeUndefined();
    expect(rangeFor({ when: "dates", from: "2026-12-24", to: "2026-12-01" }, now)).toBeUndefined();
  });
  it("ignores typed dates for a preset", () => {
    expect(rangeFor({ when: "month", from: "2020-01-01", to: "2020-01-02" }, now)?.from).toBe(
      "2026-11-04",
    );
  });
});

describe("a show's day and time at the venue", () => {
  beforeEach(() => void i18n.activate("en", ["en-GB"]));
  afterEach(() => void i18n.activate("en", ["en"]));

  it("reads the calendar day in the venue's zone, not the viewer's", () => {
    // 23:30 in New York on the 14th is already the 15th in UTC.
    const late = show("2027-04-15T03:30:00Z", "America/New_York");
    expect(showDay(late)).toBe("2027-04-14");
    expect(showDate(late)).toBe("14");
    // 00:30 on the 15th in Tokyo is still the 14th in UTC.
    const early = show("2027-04-14T15:30:00Z", "Asia/Tokyo");
    expect(showDay(early)).toBe("2027-04-15");
    expect(showWeekday(early)).toBe("Thu");
  });

  it("sets the time in the user's locale", () => {
    // London in April is on summer time, one hour ahead of UTC.
    const london = show("2027-04-14T17:00:00Z", "Europe/London");
    const here = Intl.DateTimeFormat().resolvedOptions().timeZone;
    // Only compare the clock when this computer shares the venue's offset;
    // a show elsewhere carries its zone's name.
    const time = showTime(london);
    if (here === "Europe/London") expect(time).toBe("18:00");
    else expect(time).toMatch(/18:00/);
  });

  it("names the zone when the venue's clock is not this computer's", () => {
    const here = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const far = here === "Pacific/Auckland" ? "America/Los_Angeles" : "Pacific/Auckland";
    expect(showTime(show("2027-04-14T17:00:00Z", far))).toMatch(/[A-Z]{3,5}|GMT|UTC/);
  });
});

describe("groupShows", () => {
  beforeEach(() => void i18n.activate("en", ["en-GB"]));
  afterEach(() => void i18n.activate("en", ["en"]));

  it("groups by the venue's month and day, keeping the order", () => {
    const groups = groupShows([
      show("2027-04-29T18:00:00Z", "Europe/London", "a"),
      show("2027-04-29T20:00:00Z", "Europe/London", "b"),
      // 23:30 on 30 April in London is 1 May in Tokyo, but the venue decides.
      show("2027-04-30T22:30:00Z", "Europe/London", "c"),
      show("2027-05-01T19:00:00Z", "Europe/London", "d"),
    ]);
    expect(groups.map((m) => m.title)).toEqual(["April 2027", "May 2027"]);
    expect(groups[0].days.map((d) => [d.day, d.shows.map((s) => s.id)])).toEqual([
      ["2027-04-29", ["a", "b"]],
      ["2027-04-30", ["c"]],
    ]);
    expect(groups[1].days[0].shows.map((s) => s.id)).toEqual(["d"]);
  });

  it("is empty for no shows", () => {
    expect(groupShows([])).toEqual([]);
  });
});

describe("distanceLabel", () => {
  afterEach(() => void i18n.activate("en", ["en"]));
  it("says kilometres, or miles where the locale uses them", () => {
    i18n.activate("en", ["en-GB"]);
    expect(distanceLabel(16)).toBe("10 mi away");
    i18n.activate("en", ["de-DE"]);
    expect(distanceLabel(16)).toMatch(/^16\s?km away$/);
  });
  it("says nothing for an unknown distance", () => {
    expect(distanceLabel(0)).toBe("");
  });
});
