import { FRAGMENT_SHADER, i420ToRgba, type Frame } from "./video-frames";

/** Draws pictures onto a canvas sized to each picture. */
export interface FrameRenderer {
  draw: (frame: Frame) => void;
  /** Blanks the canvas (the surface was cleared). */
  clear: () => void;
  dispose: () => void;
}

const VERTEX_SHADER = `
attribute vec2 a_pos;
varying vec2 v_uv;
void main() {
  v_uv = vec2((a_pos.x + 1.0) * 0.5, (1.0 - a_pos.y) * 0.5);
  gl_Position = vec4(a_pos, 0.0, 1.0);
}
`;

/** Plane order: texture unit and sampler name. */
const PLANES = ["u_y", "u_u", "u_v"] as const;

function resize(canvas: HTMLCanvasElement, frame: Frame): boolean {
  if (canvas.width === frame.width && canvas.height === frame.height) return false;
  canvas.width = frame.width;
  canvas.height = frame.height;
  return true;
}

function compile(gl: WebGLRenderingContext, type: number, source: string): WebGLShader | null {
  const shader = gl.createShader(type);
  if (!shader) return null;
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  return gl.getShaderParameter(shader, gl.COMPILE_STATUS) ? shader : null;
}

/** The program and its three luminance textures, or null when the context refuses them. */
function setUp(gl: WebGLRenderingContext): WebGLTexture[] | null {
  const vs = compile(gl, gl.VERTEX_SHADER, VERTEX_SHADER);
  const fs = compile(gl, gl.FRAGMENT_SHADER, FRAGMENT_SHADER);
  const program = gl.createProgram();
  if (!vs || !fs || !program) return null;
  gl.attachShader(program, vs);
  gl.attachShader(program, fs);
  gl.linkProgram(program);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) return null;
  gl.useProgram(program);

  // One quad over the whole canvas, as a strip.
  gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer());
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW);
  const pos = gl.getAttribLocation(program, "a_pos");
  gl.enableVertexAttribArray(pos);
  gl.vertexAttribPointer(pos, 2, gl.FLOAT, false, 0, 0);

  // Planes are tightly packed: rows of odd widths are not 4-byte aligned.
  gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
  const textures: WebGLTexture[] = [];
  for (const [unit, name] of PLANES.entries()) {
    const tex = gl.createTexture();
    if (!tex) return null;
    gl.activeTexture(gl.TEXTURE0 + unit);
    gl.bindTexture(gl.TEXTURE_2D, tex);
    // Not a power of two: no mipmaps, clamped edges.
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.uniform1i(gl.getUniformLocation(program, name), unit);
    textures.push(tex);
  }
  return textures;
}

/**
 * WebGL: the planes go up as three LUMINANCE textures and the fragment
 * shader converts them (BT.709, limited range). Null when the canvas has
 * no WebGL or the shaders don't build.
 */
function webglRenderer(canvas: HTMLCanvasElement): FrameRenderer | null {
  const gl = canvas.getContext("webgl", {
    alpha: false,
    antialias: false,
    depth: false,
    stencil: false,
    preserveDrawingBuffer: false,
  });
  if (!gl) return null;
  let textures = setUp(gl);
  if (!textures) return null;
  let last: Frame | null = null;

  const draw = (frame: Frame) => {
    last = frame;
    if (!textures || gl.isContextLost()) return;
    if (resize(canvas, frame)) gl.viewport(0, 0, frame.width, frame.height);
    const planes = [frame.y, frame.u, frame.v];
    const cw = Math.ceil(frame.width / 2);
    const ch = Math.ceil(frame.height / 2);
    textures.forEach((tex, unit) => {
      gl.activeTexture(gl.TEXTURE0 + unit);
      gl.bindTexture(gl.TEXTURE_2D, tex);
      const [w, h] = unit === 0 ? [frame.width, frame.height] : [cw, ch];
      gl.texImage2D(
        gl.TEXTURE_2D,
        0,
        gl.LUMINANCE,
        w,
        h,
        0,
        gl.LUMINANCE,
        gl.UNSIGNED_BYTE,
        planes[unit],
      );
    });
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
  };
  // A lost context (the GPU process restarted) comes back empty: build it
  // again and redraw the picture on show.
  const onLost = (e: Event) => e.preventDefault();
  const onRestored = () => {
    textures = setUp(gl);
    if (last) {
      canvas.width = 0;
      draw(last);
    }
  };
  canvas.addEventListener("webglcontextlost", onLost);
  canvas.addEventListener("webglcontextrestored", onRestored);

  return {
    draw,
    clear: () => {
      last = null;
      gl.clearColor(0, 0, 0, 1);
      gl.clear(gl.COLOR_BUFFER_BIT);
    },
    // The context goes with the canvas; losing it here would leave a
    // remounted view (React's development double effects) a dead context.
    dispose: () => {
      last = null;
      canvas.removeEventListener("webglcontextlost", onLost);
      canvas.removeEventListener("webglcontextrestored", onRestored);
    },
  };
}

/** Without WebGL: the same conversion in JavaScript into a 2D canvas. */
function canvasRenderer(canvas: HTMLCanvasElement): FrameRenderer | null {
  const ctx = canvas.getContext("2d", { alpha: false });
  if (!ctx) return null;
  let image: ImageData | null = null;
  return {
    draw: (frame) => {
      if (resize(canvas, frame) || !image) image = ctx.createImageData(frame.width, frame.height);
      i420ToRgba(frame, image.data);
      ctx.putImageData(image, 0, 0);
    },
    clear: () => ctx.clearRect(0, 0, canvas.width, canvas.height),
    dispose: () => {
      image = null;
    },
  };
}

/** A renderer for `canvas`: WebGL where the web view has it, else the 2D canvas; null with neither. */
export function createRenderer(canvas: HTMLCanvasElement): FrameRenderer | null {
  return webglRenderer(canvas) ?? canvasRenderer(canvas);
}
