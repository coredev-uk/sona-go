import { describe, expect, it } from "vitest";

import { settingsSearch } from "./search-params";

describe("settingsSearch", () => {
  it("keeps a section that exists", () => {
    expect(settingsSearch.parse({ section: "general" })).toEqual({ section: "general" });
  });

  it("leaves a missing, unknown or retired section to the page's last one", () => {
    expect(settingsSearch.parse({}).section).toBeUndefined();
    expect(settingsSearch.parse({ section: "nope" }).section).toBeUndefined();
    expect(settingsSearch.parse({ section: "desktop" }).section).toBeUndefined();
  });
});
