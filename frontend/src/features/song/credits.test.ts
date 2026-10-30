import { describe, expect, it } from "vitest";

import { creditTitle } from "./credits";

describe("creditTitle", () => {
  it("uses Sona's words for Apple's known kinds", () => {
    expect(creditTitle({ kind: "performer", title: "PERFORMING ARTISTS" })).toBe("Performers");
    expect(creditTitle({ kind: "composer-and-lyrics", title: "COMPOSITION & LYRICS" })).toBe(
      "Composition and lyrics",
    );
  });

  it("puts unknown kinds' titles in sentence case", () => {
    expect(creditTitle({ kind: "other", title: "MIXING & MASTERING" })).toBe(
      "Mixing and mastering",
    );
    expect(creditTitle({ kind: "other", title: "" })).toBe("Credits");
  });
});
