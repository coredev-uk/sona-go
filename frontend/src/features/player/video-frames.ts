/**
 * The music video's pictures, as Go serves them (internal/playback/video,
 * Surface.ServeHTTP): `GET /video/frame?after=<seq>` long-polls up to a
 * second for a picture newer than `seq` (0: any). A 200 carries
 * `X-Video-Seq` and a 16-byte header, then the I420 planes; a 204 means no
 * new picture came within the second, or, with `X-Video-Seq`, that the
 * surface was cleared (nothing with a picture plays) and the sequence to
 * wait from. Pictures arrive already timed to the audio: each is drawn at
 * the next animation frame, and the next is asked for only after that, so
 * a window that isn't drawing (minimised, on another workspace) neither
 * fetches nor uploads pictures.
 */

export const FRAME_PATH = "/video/frame";

/** The response header naming the picture's (or the clear's) sequence number. */
const SEQ_HEADER = "X-Video-Seq";

/** Size of the header before a picture's planes. */
export const HEADER_BYTES = 16;

/** "SVF1": the header's first four bytes. */
const MAGIC = [0x53, 0x56, 0x46, 0x31];

/** How long the loop waits after a failed request before asking again. */
const RETRY_MS = 1000;

/**
 * A picture that waited longer than this for an animation frame is stale
 * (the window wasn't drawing): it is dropped and the one on show now is
 * fetched instead.
 */
export const STALE_MS = 250;

export interface FrameHeader {
  /** Sequence number (uint32, wrapping); the request uses X-Video-Seq instead. */
  seq: number;
  width: number;
  height: number;
  /** Where the picture sits on the video's timeline. */
  positionMs: number;
}

/** One plane of an I420 picture: where it starts in the body, and its size. */
export interface Plane {
  offset: number;
  width: number;
  height: number;
}

export interface PlaneLayout {
  y: Plane;
  u: Plane;
  v: Plane;
  /** The whole body: header and planes. */
  bytes: number;
}

/** A decoded picture: its header and the three planes, viewing the response body. */
export interface Frame extends FrameHeader {
  y: Uint8Array;
  u: Uint8Array;
  v: Uint8Array;
}

/**
 * Where the planes of a width×height I420 picture sit in a response body:
 * luma at full size after the header, then U and V at ⌈w/2⌉×⌈h/2⌉ each,
 * tightly packed.
 */
export function planeLayout(width: number, height: number): PlaneLayout {
  const cw = Math.ceil(width / 2);
  const ch = Math.ceil(height / 2);
  const luma = width * height;
  const chroma = cw * ch;
  return {
    y: { offset: HEADER_BYTES, width, height },
    u: { offset: HEADER_BYTES + luma, width: cw, height: ch },
    v: { offset: HEADER_BYTES + luma + chroma, width: cw, height: ch },
    bytes: HEADER_BYTES + luma + 2 * chroma,
  };
}

/** The header of a picture body (little-endian), or null when it isn't one. */
export function parseFrameHeader(body: ArrayBuffer): FrameHeader | null {
  if (body.byteLength < HEADER_BYTES) return null;
  const view = new DataView(body);
  if (MAGIC.some((b, i) => view.getUint8(i) !== b)) return null;
  return {
    seq: view.getUint32(4, true),
    width: view.getUint16(8, true),
    height: view.getUint16(10, true),
    positionMs: view.getUint32(12, true),
  };
}

/** A picture body as a Frame, or null when its header is wrong or its planes are short. */
export function parseFrame(body: ArrayBuffer): Frame | null {
  const header = parseFrameHeader(body);
  if (!header || header.width === 0 || header.height === 0) return null;
  const layout = planeLayout(header.width, header.height);
  if (body.byteLength < layout.bytes) return null;
  const plane = (p: Plane) => new Uint8Array(body, p.offset, p.width * p.height);
  return { ...header, y: plane(layout.y), u: plane(layout.u), v: plane(layout.v) };
}

/**
 * What one request answered, and the `after` the next request sends:
 * - `picture`: a new picture to draw (`after` is its X-Video-Seq);
 * - `waiting`: nothing new within the second; keep the picture on show;
 * - `cleared`: the surface was cleared; take the picture down and wait
 *   from the sequence Go named;
 * - `failed`: the request failed or the body was not a picture; ask again
 *   after a pause, from the same place.
 */
export type FrameAnswer =
  | { kind: "picture"; after: number; frame: Frame }
  | { kind: "waiting"; after: number }
  | { kind: "cleared"; after: number }
  | { kind: "failed"; after: number };

/** X-Video-Seq as a number, or null when absent or not a sequence number. */
function seqOf(header: string | null): number | null {
  if (!header || !/^\d+$/.test(header)) return null;
  const n = Number(header);
  return Number.isSafeInteger(n) ? n : null;
}

/** Reads one response (its status, X-Video-Seq and, for a 200, the body) sent with `after`. */
export function readAnswer(
  status: number,
  seqHeader: string | null,
  body: ArrayBuffer | null,
  after: number,
): FrameAnswer {
  const seq = seqOf(seqHeader);
  if (status === 200) {
    const frame = body ? parseFrame(body) : null;
    if (!frame || seq === null) return { kind: "failed", after };
    return { kind: "picture", after: seq, frame };
  }
  if (status === 204) {
    return seq === null ? { kind: "waiting", after } : { kind: "cleared", after: seq };
  }
  return { kind: "failed", after };
}

export interface FrameLoopHandlers {
  onPicture: (frame: Frame) => void;
  onWaiting: () => void;
  onCleared: () => void;
}

type Fetch = (url: string, init: RequestInit) => Promise<Response>;

/** Waits for the next animation frame (or `signal`), resolving with how many milliseconds it took. */
type NextFrame = (signal: AbortSignal) => Promise<number>;

function animationFrame(signal: AbortSignal): Promise<number> {
  const start = performance.now();
  return new Promise((resolve) => {
    const done = () => resolve(performance.now() - start);
    const id = requestAnimationFrame(done);
    signal.addEventListener(
      "abort",
      () => {
        cancelAnimationFrame(id);
        done();
      },
      { once: true },
    );
  });
}

/** Resolves after `ms`, or at once when `signal` aborts. */
function pause(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const id = setTimeout(resolve, ms);
    signal.addEventListener(
      "abort",
      () => {
        clearTimeout(id);
        resolve();
      },
      { once: true },
    );
  });
}

/**
 * Asks Go for pictures until `signal` aborts, one request in flight at a
 * time, handing each answer to `handlers`; a picture waits for the next
 * animation frame (`nextFrame`). Starts from 0 (whatever is on show).
 */
export async function runFrameLoop(
  signal: AbortSignal,
  handlers: FrameLoopHandlers,
  fetchFn: Fetch = (url, init) => fetch(url, init),
  nextFrame: NextFrame = animationFrame,
): Promise<void> {
  let after = 0;
  while (!signal.aborted) {
    let answer: FrameAnswer;
    try {
      const res = await fetchFn(`${FRAME_PATH}?after=${after}`, { signal, cache: "no-store" });
      const body = res.status === 200 ? await res.arrayBuffer() : null;
      answer = readAnswer(res.status, res.headers.get(SEQ_HEADER), body, after);
    } catch {
      answer = { kind: "failed", after };
    }
    if (signal.aborted) return;
    switch (answer.kind) {
      case "picture": {
        const waited = await nextFrame(signal);
        if (signal.aborted) return;
        // Stale: ask again from the same place, which answers at once
        // with the picture on show now (this one, if nothing newer came).
        if (waited > STALE_MS) continue;
        handlers.onPicture(answer.frame);
        break;
      }
      case "waiting":
        handlers.onWaiting();
        break;
      case "cleared":
        handlers.onCleared();
        break;
      case "failed":
        await pause(RETRY_MS, signal);
        break;
    }
    after = answer.after;
  }
}

/**
 * The largest box of the picture's shape that fits `box`: the picture is
 * letterboxed, never cropped or stretched.
 */
export function fitPicture(
  picture: { width: number; height: number },
  box: { width: number; height: number },
): { width: number; height: number } {
  if (picture.width <= 0 || picture.height <= 0 || box.width <= 0 || box.height <= 0) {
    return { width: 0, height: 0 };
  }
  const scale = Math.min(box.width / picture.width, box.height / picture.height);
  return {
    width: Math.floor(picture.width * scale),
    height: Math.floor(picture.height * scale),
  };
}

/*
 * BT.709, limited range (Y 16–235, Cb/Cr 16–240) to full-range RGB: luma
 * scaled by 255/219, chroma by 255/224 into the matrix's coefficients.
 */
const Y_SCALE = 1.1643836;
const R_V = 1.7927411;
const G_U = 0.2132486;
const G_V = 0.5329093;
const B_U = 2.1124018;

/** The fragment shader's conversion, for the 2D canvas when WebGL is unavailable. */
export function i420ToRgba(frame: Frame, out: Uint8ClampedArray): void {
  const { width, height, y, u, v } = frame;
  const cw = Math.ceil(width / 2);
  let o = 0;
  for (let row = 0; row < height; row++) {
    const lumaRow = row * width;
    const chromaRow = (row >> 1) * cw;
    for (let col = 0; col < width; col++) {
      const c = Y_SCALE * (y[lumaRow + col] - 16);
      const ci = chromaRow + (col >> 1);
      const d = u[ci] - 128;
      const e = v[ci] - 128;
      out[o] = c + R_V * e;
      out[o + 1] = c - G_U * d - G_V * e;
      out[o + 2] = c + B_U * d;
      out[o + 3] = 255;
      o += 4;
    }
  }
}

/** The same conversion in GLSL, on three single-channel textures. */
export const FRAGMENT_SHADER = `
precision mediump float;
varying vec2 v_uv;
uniform sampler2D u_y;
uniform sampler2D u_u;
uniform sampler2D u_v;
void main() {
  float y = ${Y_SCALE} * (texture2D(u_y, v_uv).r - 0.0627451);
  float u = texture2D(u_u, v_uv).r - 0.5019608;
  float v = texture2D(u_v, v_uv).r - 0.5019608;
  gl_FragColor = vec4(y + ${R_V} * v, y - ${G_U} * u - ${G_V} * v, y + ${B_U} * u, 1.0);
}
`;
