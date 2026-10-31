import { beforeEach, describe, expect, it } from "vitest";

import { privateModule } from "./index";
import { applyPrivate, usePrivate } from "./state";

const command = privateModule.shortcuts?.find((s) => s.id === "private.toggle");

describe("private listening palette command", () => {
  beforeEach(() => usePrivate.setState({ state: null }));

  it("is unavailable until Go reports the state", () => {
    expect(command?.when?.()).toBe(false);
  });

  it("names the action that the current state allows", () => {
    applyPrivate({ on: false, source: "setting", saved: false });
    expect(command?.label()).toBe("Turn on private listening");
    expect(command?.when?.()).toBe(true);
    applyPrivate({ on: true, source: "development", saved: false });
    expect(command?.label()).toBe("Turn off private listening");
    expect(command?.when?.()).toBe(true);
  });

  it("is unavailable while SONA_PRIVATE=1 forces it on", () => {
    applyPrivate({ on: true, source: "environment", saved: false });
    expect(command?.when?.()).toBe(false);
  });
});
