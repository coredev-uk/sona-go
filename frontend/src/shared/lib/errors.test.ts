import { ErrorCode } from "@bindings/services/models";
import { describe, expect, it } from "vitest";

import { callFailure, codeMessage } from "./errors";

const generic = callFailure({ cause: { code: "unknown", message: "x" } }).message;

describe("callFailure", () => {
  it("words every services error code, never with Go's text", () => {
    const codes = Object.values(ErrorCode);
    expect(codes.length).toBeGreaterThan(10);
    for (const code of codes) {
      const goText = `services: Go's own text for ${code}`;
      const f = callFailure({ cause: { code, message: goText } });
      expect(f.message, code).toBeTruthy();
      expect(f.message, code).not.toContain("Go's own text");
      expect(f.detail, code).toBe(`Go's own text for ${code}`);
      // Only unknown (and no code) share the generic wording.
      if (code !== ErrorCode.CodeUnknown && code !== ErrorCode.$zero) {
        expect(f.message, code).not.toBe(generic);
      }
    }
  });

  it("gives unknown and unrecognised codes the generic message", () => {
    expect(generic).toBe("Something went wrong.");
    const unknown = callFailure({ cause: { code: "unknown", message: "library: disk full" } });
    expect(unknown.message).toBe(generic);
    expect(unknown.detail).toBe("disk full");
    expect(callFailure({ cause: { code: "made_up", message: "x" } }).message).toBe(generic);
    expect(callFailure(new Error("boom")).message).toBe(generic);
    expect(callFailure(new Error("boom")).detail).toBe("boom");
  });

  it("words codes sent in DTOs", () => {
    expect(codeMessage(ErrorCode.CodeKeyring)).toBe(
      callFailure({ cause: { code: "keyring", message: "" } }).message,
    );
    expect(codeMessage("made_up")).toBe(generic);
  });
});
