import { describe, expect, it } from "vitest";

import { concertsSearch } from "./search-params";

describe("concertsSearch", () => {
  it("keeps what it can read", () => {
    expect(
      concertsSearch.parse({
        when: "dates",
        from: "2027-04-01",
        to: "2027-04-30",
        genre: "20",
        yours: true,
      }),
    ).toEqual({ when: "dates", from: "2027-04-01", to: "2027-04-30", genre: "20", yours: true });
  });

  it("drops what it cannot, from a saved or hand-typed address", () => {
    expect(
      concertsSearch.parse({
        when: "tomorrow",
        from: "April",
        to: "2027-4-1",
        genre: "rock",
        yours: "no",
      }),
    ).toEqual({});
  });
});
