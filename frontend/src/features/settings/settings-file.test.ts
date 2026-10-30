import { describe, expect, it } from "vitest";

import { changedRows } from "./settings-file";

describe("changedRows", () => {
  it("names a group's fields once by their row", () => {
    expect(
      changedRows([
        "audio.equalizer.custom",
        "audio.equalizer.enabled",
        "audio.equalizer.preset",
        "desktop.closeAction",
        "theme",
      ]),
    ).toEqual(["Equaliser", "When the window is closed", "Theme"]);
  });

  it("matches a nested name to its group's row", () => {
    expect(changedRows(["crossfade.seconds"])).toEqual(["Crossfade"]);
  });

  it("keeps a name it has no row for", () => {
    expect(changedRows(["future.setting"])).toEqual(["future.setting"]);
  });
});
