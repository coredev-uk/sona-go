import { describe, expect, it } from "vitest";

import type { QueueItem } from "@bindings/services";

import { isCurrent } from "./now-playing";

describe("isCurrent", () => {
  const current = { id: "i.abc", catalogId: "1097862703" } as QueueItem;

  it("matches the same id or the same catalogue song", () => {
    expect(isCurrent({ id: "i.abc", catalogId: "" }, current)).toBe(true);
    // The catalogue copy of the playing library song is marked too.
    expect(isCurrent({ id: "1097862703", catalogId: "1097862703" }, current)).toBe(true);
    expect(isCurrent({ id: "i.xyz", catalogId: "" }, current)).toBe(false);
    expect(isCurrent({ id: "i.abc", catalogId: "" }, null)).toBe(false);
  });
});
