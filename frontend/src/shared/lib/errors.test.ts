import { readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { callFailure, hasCodeMessage } from "./errors";

describe("callFailure", () => {
  it("has wording for every services error code except unknown", () => {
    const src = readFileSync(
      path.resolve(import.meta.dirname, "../../../../internal/services/errors.go"),
      "utf8",
    );
    const codes = [...src.matchAll(/^\s*Code\w+\s*=\s*"([a-z0-9_]+)"/gm)].map((m) => m[1]);
    expect(codes.length).toBeGreaterThan(10);
    expect(codes.filter((c) => c !== "unknown" && !hasCodeMessage(c))).toEqual([]);
  });

  it("translates known codes and keeps Go's text for unknown ones", () => {
    const known = callFailure({ cause: { code: "not_playable", message: "applemusic: gone" } });
    expect(known.message).not.toBe("gone");
    expect(known.detail).toBe("gone");
    const unknown = callFailure({ cause: { code: "unknown", message: "library: disk full" } });
    expect(unknown.message).toBe("disk full");
    expect(callFailure(new Error("boom")).message).toBe("boom");
  });
});
