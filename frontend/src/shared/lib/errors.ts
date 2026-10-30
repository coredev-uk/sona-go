/** A rejected binding call, reduced to Go's stable code and its message. */
export interface CallFailure {
  /** Services error code from `err.cause.code` (internal/services/errors.go), or "". */
  code: string;
  /** Go's error text without its package prefix ("appleid: …"). */
  message: string;
}

function field(obj: unknown, key: string): unknown {
  return obj && typeof obj === "object" && key in obj
    ? (obj as Record<string, unknown>)[key]
    : undefined;
}

/**
 * Reads a Wails call rejection: the Go service marshals errors to
 * `err.cause = {code, message}`; anything else falls back to its message.
 */
export function callFailure(err: unknown): CallFailure {
  if (!err) return { code: "", message: "" };
  const cause = field(err, "cause");
  const code = field(cause, "code");
  const causeMsg = field(cause, "message");
  const msg = field(err, "message");
  const raw =
    typeof causeMsg === "string" && causeMsg
      ? causeMsg
      : typeof msg === "string"
        ? msg
        : typeof err === "string"
          ? err
          : String(err);
  return {
    code: typeof code === "string" ? code : "",
    message: raw.replace(/^[a-z]+: /, ""),
  };
}
