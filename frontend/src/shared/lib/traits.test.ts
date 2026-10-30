import { describe, expect, it } from "vitest";

import { traitLabels } from "./traits";

const none = {
  lossless: false,
  hiResLossless: false,
  atmos: false,
  spatial: false,
  appleDigitalMaster: false,
};

describe("traitLabels", () => {
  it("names the most specific label of each kind", () => {
    expect(
      traitLabels({ ...none, lossless: true, hiResLossless: true, atmos: true, spatial: true }),
    ).toEqual(["Dolby Atmos", "Hi-Res Lossless"]);
    expect(
      traitLabels({ ...none, spatial: true, lossless: true, appleDigitalMaster: true }),
    ).toEqual(["Spatial Audio", "Lossless", "Apple Digital Master"]);
  });

  it("is empty without traits", () => {
    expect(traitLabels(none)).toEqual([]);
    expect(traitLabels(undefined)).toEqual([]);
  });
});
