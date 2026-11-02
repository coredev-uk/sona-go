import { describe, expect, it, vi } from "vitest";

import {
  fitPicture,
  HEADER_BYTES,
  i420ToRgba,
  parseFrame,
  parseFrameHeader,
  planeLayout,
  readAnswer,
  runFrameLoop,
  STALE_MS,
  type Frame,
} from "./video-frames";

/** A picture body as Go writes it (surface.go encodeHeader + planes), each plane filled with one value. */
function body(
  seq: number,
  width: number,
  height: number,
  positionMs: number,
  fill = { y: 16, u: 128, v: 128 },
): ArrayBuffer {
  const layout = planeLayout(width, height);
  const buf = new ArrayBuffer(layout.bytes);
  const bytes = new Uint8Array(buf);
  bytes.set([0x53, 0x56, 0x46, 0x31]); // "SVF1"
  const view = new DataView(buf);
  view.setUint32(4, seq, true);
  view.setUint16(8, width, true);
  view.setUint16(10, height, true);
  view.setUint32(12, positionMs, true);
  bytes.fill(fill.y, layout.y.offset, layout.u.offset);
  bytes.fill(fill.u, layout.u.offset, layout.v.offset);
  bytes.fill(fill.v, layout.v.offset, layout.bytes);
  return buf;
}

function response(status: number, seq?: string, data?: ArrayBuffer): Response {
  const headers = new Headers();
  if (seq !== undefined) headers.set("X-Video-Seq", seq);
  return new Response(status === 204 ? null : data, { status, headers });
}

describe("planeLayout", () => {
  it("puts luma after the header, then U and V at half size", () => {
    expect(planeLayout(864, 486)).toEqual({
      y: { offset: 16, width: 864, height: 486 },
      u: { offset: 16 + 864 * 486, width: 432, height: 243 },
      v: { offset: 16 + 864 * 486 + 432 * 243, width: 432, height: 243 },
      bytes: 16 + 864 * 486 + 2 * 432 * 243,
    });
  });

  it("rounds odd chroma sizes up", () => {
    const l = planeLayout(5, 3);
    expect(l.u).toEqual({ offset: 16 + 15, width: 3, height: 2 });
    expect(l.v).toEqual({ offset: 16 + 15 + 6, width: 3, height: 2 });
    expect(l.bytes).toBe(16 + 15 + 12);
  });
});

describe("parseFrameHeader", () => {
  it("reads the little-endian fields after SVF1", () => {
    expect(parseFrameHeader(body(0x01020304, 864, 486, 61_250))).toEqual({
      seq: 0x01020304,
      width: 864,
      height: 486,
      positionMs: 61_250,
    });
  });

  it("refuses a body without the magic or shorter than the header", () => {
    const wrong = body(1, 2, 2, 0);
    new Uint8Array(wrong)[3] = 0x32; // "SVF2"
    expect(parseFrameHeader(wrong)).toBeNull();
    expect(parseFrameHeader(new ArrayBuffer(HEADER_BYTES - 1))).toBeNull();
  });
});

describe("parseFrame", () => {
  it("views the three planes of the body", () => {
    const f = parseFrame(body(7, 4, 2, 0, { y: 100, u: 90, v: 80 }))!;
    expect(f.y).toHaveLength(8);
    expect(f.u).toHaveLength(2);
    expect(f.v).toHaveLength(2);
    expect([f.y[0], f.u[0], f.v[1]]).toEqual([100, 90, 80]);
  });

  it("refuses short planes and an empty picture", () => {
    expect(parseFrame(body(7, 4, 2, 0).slice(0, -1))).toBeNull();
    expect(parseFrame(body(7, 0, 2, 0))).toBeNull();
  });
});

describe("readAnswer", () => {
  it("a 200 is a picture; the next request waits from its X-Video-Seq", () => {
    const a = readAnswer(200, "4294967297", body(1, 2, 2, 0), 0);
    expect(a.kind).toBe("picture");
    // The header's seq wraps at 32 bits; X-Video-Seq does not.
    expect(a.after).toBe(4294967297);
  });

  it("a 204 without X-Video-Seq timed out: keep the picture and the place", () => {
    expect(readAnswer(204, null, null, 41)).toEqual({ kind: "waiting", after: 41 });
  });

  it("a 204 with X-Video-Seq is a cleared surface: wait from that sequence", () => {
    expect(readAnswer(204, "42", null, 41)).toEqual({ kind: "cleared", after: 42 });
  });

  it("anything else fails and keeps the place", () => {
    expect(readAnswer(500, null, null, 41)).toEqual({ kind: "failed", after: 41 });
    expect(readAnswer(200, null, body(1, 2, 2, 0), 41)).toEqual({ kind: "failed", after: 41 });
    expect(readAnswer(200, "42", new ArrayBuffer(4), 41)).toEqual({ kind: "failed", after: 41 });
  });
});

describe("runFrameLoop", () => {
  it("asks one picture at a time, from the last sequence it heard", async () => {
    const abort = new AbortController();
    const urls: string[] = [];
    let inFlight = 0;
    let most = 0;
    const answers = [
      response(200, "5", body(5, 2, 2, 40)),
      response(204),
      response(204, "6"),
      response(200, "7", body(7, 2, 2, 80)),
    ];
    const fetchFn = vi.fn(async (url: string) => {
      urls.push(url);
      inFlight++;
      most = Math.max(most, inFlight);
      await Promise.resolve();
      inFlight--;
      const next = answers.shift();
      if (!next) abort.abort();
      return next ?? response(204);
    });
    const pictures: number[] = [];
    const events: string[] = [];
    await runFrameLoop(
      abort.signal,
      {
        onPicture: (f: Frame) => {
          pictures.push(f.positionMs);
          events.push("picture");
        },
        onWaiting: () => events.push("waiting"),
        onCleared: () => events.push("cleared"),
      },
      fetchFn,
      async () => 0,
    );
    expect(urls).toEqual([
      "/video/frame?after=0",
      "/video/frame?after=5",
      "/video/frame?after=5",
      "/video/frame?after=6",
      "/video/frame?after=7",
    ]);
    expect(events).toEqual(["picture", "waiting", "cleared", "picture"]);
    expect(pictures).toEqual([40, 80]);
    expect(most).toBe(1);
  });

  it("draws at the next animation frame, asking for the next picture only after it", async () => {
    const abort = new AbortController();
    const log: string[] = [];
    const answers = [response(200, "5", body(5, 2, 2, 40)), response(200, "6", body(6, 2, 2, 80))];
    const fetchFn = vi.fn(async (url: string) => {
      log.push(url);
      const next = answers.shift();
      if (!next) abort.abort();
      return next ?? response(204);
    });
    await runFrameLoop(
      abort.signal,
      {
        onPicture: (f) => log.push(`draw ${f.positionMs}`),
        onWaiting: vi.fn(),
        onCleared: vi.fn(),
      },
      fetchFn,
      async () => {
        log.push("frame");
        return 16;
      },
    );
    expect(log).toEqual([
      "/video/frame?after=0",
      "frame",
      "draw 40",
      "/video/frame?after=5",
      "frame",
      "draw 80",
      "/video/frame?after=6",
    ]);
  });

  it("drops a picture that waited too long for a frame and asks again from the same place", async () => {
    const abort = new AbortController();
    const urls: string[] = [];
    const answers = [response(200, "5", body(5, 2, 2, 40)), response(200, "9", body(9, 2, 2, 360))];
    const fetchFn = vi.fn(async (url: string) => {
      urls.push(url);
      const next = answers.shift();
      if (!next) abort.abort();
      return next ?? response(204);
    });
    const waits = [STALE_MS + 1, 16];
    const drawn: number[] = [];
    await runFrameLoop(
      abort.signal,
      { onPicture: (f) => drawn.push(f.positionMs), onWaiting: vi.fn(), onCleared: vi.fn() },
      fetchFn,
      async () => waits.shift() ?? 16,
    );
    expect(urls).toEqual(["/video/frame?after=0", "/video/frame?after=0", "/video/frame?after=9"]);
    expect(drawn).toEqual([360]);
  });

  it("stops when aborted mid-request, without handling the answer", async () => {
    const abort = new AbortController();
    const onPicture = vi.fn();
    const fetchFn = vi.fn((_url: string, init: RequestInit) => {
      return new Promise<Response>((_, reject) => {
        init.signal?.addEventListener("abort", () => reject(new DOMException("", "AbortError")));
      });
    });
    const done = runFrameLoop(
      abort.signal,
      { onPicture, onWaiting: vi.fn(), onCleared: vi.fn() },
      fetchFn,
      async () => 0,
    );
    abort.abort();
    await done;
    expect(fetchFn).toHaveBeenCalledTimes(1);
    expect(onPicture).not.toHaveBeenCalled();
  });
});

describe("fitPicture", () => {
  it("letterboxes into a wider box and pillarboxes into a taller one", () => {
    expect(fitPicture({ width: 864, height: 486 }, { width: 2000, height: 900 })).toEqual({
      width: 1600,
      height: 900,
    });
    expect(fitPicture({ width: 864, height: 486 }, { width: 800, height: 900 })).toEqual({
      width: 800,
      height: 450,
    });
  });

  it("is empty before the box is measured", () => {
    expect(fitPicture({ width: 16, height: 9 }, { width: 0, height: 0 })).toEqual({
      width: 0,
      height: 0,
    });
  });
});

describe("i420ToRgba", () => {
  const convert = (y: number, u: number, v: number) => {
    const f = parseFrame(body(1, 2, 2, 0, { y, u, v }))!;
    const out = new Uint8ClampedArray(2 * 2 * 4);
    i420ToRgba(f, out);
    return [...out.slice(0, 4)];
  };

  it("maps limited-range black and white to full range", () => {
    expect(convert(16, 128, 128)).toEqual([0, 0, 0, 255]);
    expect(convert(235, 128, 128)).toEqual([255, 255, 255, 255]);
  });

  it("uses BT.709 coefficients", () => {
    // BT.709 limited-range red, green and blue (ITU-R BT.709 Y'CbCr of the primaries).
    const near = (got: number[], want: number[]) =>
      got.slice(0, 3).forEach((c, i) => expect(Math.abs(c - want[i])).toBeLessThanOrEqual(2));
    near(convert(63, 102, 240), [255, 0, 0]);
    near(convert(173, 42, 26), [0, 255, 0]);
    near(convert(32, 240, 118), [0, 0, 255]);
  });
});
