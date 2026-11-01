import { describe, expect, it } from "vitest";

import { formatTimeLeft } from "./format";
import { isUpcoming, releaseBadge } from "./release";

const release = new Date(2026, 10, 19).getTime();
const before = new Date(2026, 10, 1, 14).getTime();

describe("pre-release albums", () => {
  it("is upcoming until its release, then out", () => {
    const album = { prerelease: true, releaseAt: release };
    expect(isUpcoming(album, before)).toBe(true);
    expect(isUpcoming(album, release)).toBe(false);
    expect(isUpcoming({ prerelease: false, releaseAt: release }, before)).toBe(false);
    // Only a year: Apple's flag decides, and there is no badge.
    expect(isUpcoming({ prerelease: true, releaseAt: 0 }, before)).toBe(true);
    expect(releaseBadge({ prerelease: true, releaseAt: 0 }, before)).toBeUndefined();
  });

  it("badges the cover with the short date", () => {
    expect(releaseBadge({ prerelease: true, releaseAt: release }, before)).toMatch(/^Out .*19/);
    expect(releaseBadge({ prerelease: true, releaseAt: release }, release)).toBeUndefined();
  });

  it("counts down to the minute, never finer", () => {
    const min = 60_000;
    expect(formatTimeLeft(18 * 1440 * min + 6 * 60 * min)).toMatch(/18 days, 6 hours/);
    expect(formatTimeLeft(5 * 60 * min + 12 * min)).toMatch(/5 hours, 12 minutes/);
    expect(formatTimeLeft(20_000)).toMatch(/^1 minute$/);
  });
});
