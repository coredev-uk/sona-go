import type { ArtworkColors } from "@bindings/services";

/**
 * The album page's colours, taken from the cover: Apple's own colours for
 * it (`ArtworkColors`: the background the cover sits on and the text
 * colours Apple sets over it) and Go's dominant colours (`Header.colors`).
 *
 * - The sleeve is printed in the cover's background colour, kept one tone
 *   step off the ground, and no lighter than a mid tone in the dark theme
 *   (a white sleeve would be the brightest thing in a dark window).
 * - Text on the sleeve starts from Apple's text colours and is moved in
 *   lightness, hue kept, until it reaches 4.5:1 on the sleeve and on its
 *   hover and pressed tones; Apple's colours are used as they are when
 *   they already do.
 * - The cover's most colourful colour, when it has one, takes amber's job
 *   on the page: the play control and the playing row, and the artist's
 *   link. Its fill is never less distinct from the ground than amber's,
 *   its glyph keeps 4.5:1, and as text it keeps 4.5:1 on every row tone.
 *   A greyscale cover keeps Sona's amber.
 *
 * Everything is worked out per theme from the theme's tokens (THEMES,
 * kept equal to `shared/styles/index.css` by a test), so the result is
 * plain hex values set as custom properties: nothing is blended at paint
 * time and nothing depends on the backdrop image.
 */

/** sRGB, each channel 0–1. */
type Rgb = readonly [number, number, number];

interface Lch {
  l: number;
  c: number;
  h: number;
}

export type ThemeName = "light" | "dark";

/** The theme tokens the album's colours are checked against (`index.css`). */
export const THEMES = {
  light: {
    ground: "#f4f5f3",
    hover: "#e4e7e3",
    active: "#dbdfda",
    ink: "#161917",
    amber: "#e5a31b",
    onAmber: "#1a1405",
  },
  dark: {
    ground: "#121513",
    hover: "#222724",
    active: "#2a302c",
    ink: "#eef1ee",
    amber: "#f0b43c",
    onAmber: "#1a1405",
  },
} as const;

/** Text against its background (WCAG AA, body text). */
export const TEXT_CONTRAST = 4.5;
/** Controls and their edges against their background (WCAG AA, non-text). */
export const UI_CONTRAST = 3;
/** The sleeve stays at least this far from the ground in OKLCH lightness (one tone step). */
const SLEEVE_STEP = 0.05;
/** The sleeve's lightest tone in the dark theme. */
const DARK_SLEEVE_MAX = 0.36;
/** Below this OKLCH chroma a colour reads as grey and is no accent. */
const ACCENT_MIN_CHROMA = 0.06;
/** Muted text must still read as a step below the ink, not the same colour. */
const MUTED_STEP = 1.15;
/**
 * The contrast black or white keeps on a sleeve and its pressed tone, so
 * that ink and a muted step below it both reach 4.5:1 there.
 */
const SLEEVE_HEADROOM = 5.8;
/** The two ends a text colour can always fall back to. */
const WHITE = "#ffffff";
const BLACK = "#000000";

/** OKLCH lightness, 0–1. */
export function lightness(hex: string): number {
  return oklch(hex).l;
}

export function parseHex(hex: string): Rgb | undefined {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim());
  if (!m) return undefined;
  const n = parseInt(m[1], 16);
  return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
}

function toHex(c: Rgb): string {
  return (
    "#" +
    c
      .map((v) =>
        Math.round(Math.min(1, Math.max(0, v)) * 255)
          .toString(16)
          .padStart(2, "0"),
      )
      .join("")
  );
}

const toLinear = (v: number) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
const fromLinear = (v: number) => (v <= 0.0031308 ? v * 12.92 : 1.055 * v ** (1 / 2.4) - 0.055);

function rgb(hex: string): Rgb {
  const c = parseHex(hex);
  if (!c) throw new Error(`not a colour: ${hex}`);
  return c;
}

/** WCAG relative luminance. */
export function luminance(hex: string): number {
  const [r, g, b] = rgb(hex).map(toLinear);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** WCAG contrast ratio of two colours, 1–21. */
export function contrast(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

// OKLab (Björn Ottosson), from and to linear sRGB.
function toOklab([r, g, b]: Rgb): [number, number, number] {
  const [lr, lg, lb] = [r, g, b].map(toLinear);
  const l = Math.cbrt(0.4122214708 * lr + 0.5363325363 * lg + 0.0514459929 * lb);
  const m = Math.cbrt(0.2119034982 * lr + 0.6806995451 * lg + 0.1073969566 * lb);
  const s = Math.cbrt(0.0883024619 * lr + 0.2817188376 * lg + 0.6299787005 * lb);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}

/** Linear sRGB from OKLab; channels may fall outside 0–1 (out of gamut). */
function fromOklabLinear([L, a, b]: readonly [number, number, number]): [number, number, number] {
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
}

function oklch(hex: string): Lch {
  const [l, a, b] = toOklab(rgb(hex));
  return { l, c: Math.hypot(a, b), h: Math.atan2(b, a) };
}

const inGamut = (c: readonly number[]) => c.every((v) => v >= -1e-4 && v <= 1 + 1e-4);

/** The colour at OKLCH `lch`, its chroma reduced until it fits sRGB. */
function fromOklch({ l, c, h }: Lch): string {
  const L = Math.min(1, Math.max(0, l));
  const at = (chroma: number) => fromOklabLinear([L, chroma * Math.cos(h), chroma * Math.sin(h)]);
  let lin = at(c);
  if (!inGamut(lin)) {
    let lo = 0;
    let hi = c;
    for (let i = 0; i < 24; i++) {
      const mid = (lo + hi) / 2;
      if (inGamut(at(mid))) lo = mid;
      else hi = mid;
    }
    lin = at(lo);
  }
  return toHex(lin.map((v) => fromLinear(Math.min(1, Math.max(0, v)))) as unknown as Rgb);
}

/** `b` mixed into `a` by `t` (0–1) in OKLab, as CSS `color-mix(in oklab, a, b t)`. */
export function mix(a: string, b: string, t: number): string {
  const x = toOklab(rgb(a));
  const y = toOklab(rgb(b));
  const lin = fromOklabLinear(
    [0, 1, 2].map((i) => x[i] + (y[i] - x[i]) * t) as [number, number, number],
  );
  return toHex(lin.map((v) => fromLinear(Math.min(1, Math.max(0, v)))) as unknown as Rgb);
}

/** `fg` at `alpha` painted over `bg`, as the browser composites it (in sRGB). */
export function over(bg: string, fg: string, alpha: number): string {
  const b = rgb(bg);
  const f = rgb(fg);
  return toHex([0, 1, 2].map((i) => b[i] + (f[i] - b[i]) * alpha) as unknown as Rgb);
}

/**
 * The colour nearest `hex` in OKLCH lightness (hue and chroma kept, as far
 * as sRGB allows) for which `ok` holds; undefined when no lightness does.
 */
function nearest(hex: string, ok: (c: string) => boolean): string | undefined {
  if (ok(hex)) return hex;
  const { l, c, h } = oklch(hex);
  const steps = 200;
  for (let i = 1; i <= steps; i++) {
    const d = i / steps;
    if (l + d > 1 + 1 / steps && l - d < -1 / steps) break;
    for (const L of [l - d, l + d]) {
      if (L < 0 || L > 1) continue;
      const cand = fromOklch({ l: L, c, h });
      if (ok(cand)) return cand;
    }
  }
  for (const end of [BLACK, WHITE]) if (ok(end)) return end;
  return undefined;
}

const worst = (c: string, bgs: readonly string[]) => Math.min(...bgs.map((b) => contrast(c, b)));

/**
 * `fg`, or the nearest colour of its hue that keeps `min`:1 against every
 * background; black or white, whichever does better, when none does.
 */
export function ensureContrast(fg: string, bgs: readonly string[], min: number): string {
  return (
    nearest(fg, (c) => worst(c, bgs) >= min) ??
    (worst(BLACK, bgs) >= worst(WHITE, bgs) ? BLACK : WHITE)
  );
}

/** The most colourful of `candidates`, or undefined when all of them read as grey. */
export function accentOf(candidates: readonly string[]): string | undefined {
  let best: string | undefined;
  let chroma = ACCENT_MIN_CHROMA;
  for (const c of candidates) {
    if (!parseHex(c)) continue;
    const { c: ch } = oklch(c);
    if (ch >= chroma) {
      best = c;
      chroma = ch;
    }
  }
  return best;
}

/** The black or white that text on `surface` sits towards: whichever contrasts more. */
function poleOf(surface: string): string {
  return contrast(BLACK, surface) >= contrast(WHITE, surface) ? BLACK : WHITE;
}

/** A surface with its hover and pressed tones: steps towards the side its text sits on, as the theme's own. */
function tonesOf(surface: string): [string, string, string] {
  const pole = poleOf(surface);
  return [surface, mix(surface, pole, 0.06), mix(surface, pole, 0.1)];
}

/**
 * The sleeve's surface for a cover background `bg` in `theme`: the cover's
 * colour, one tone step off the ground, no lighter than a mid tone in the
 * dark theme, and far enough from a mid grey that text on it and on its
 * hover and pressed tones can reach 4.5:1 with room for a muted step.
 */
export function sleeveSurface(bg: string, theme: ThemeName): string {
  const lch = oklch(bg);
  const ground = oklch(THEMES[theme].ground).l;
  let l = theme === "dark" ? Math.min(lch.l, DARK_SLEEVE_MAX) : lch.l;
  if (Math.abs(l - ground) < SLEEVE_STEP) {
    l = theme === "dark" ? ground + SLEEVE_STEP : ground - SLEEVE_STEP;
  }
  const fits = (s: string) => {
    const sl = lightness(s);
    return (
      Math.abs(sl - ground) >= SLEEVE_STEP - 0.005 &&
      (theme === "light" || sl <= DARK_SLEEVE_MAX + 0.005) &&
      worst(poleOf(s), tonesOf(s)) >= SLEEVE_HEADROOM
    );
  };
  const start = fromOklch({ ...lch, l });
  return nearest(start, fits) ?? start;
}

/** The sleeve's colours: surface, its row tones, and the text and lines on it. */
export interface SleeveColors {
  surface: string;
  hover: string;
  active: string;
  ink: string;
  inkMuted: string;
  line: string;
  controlLine: string;
  /** The accent as text on the sleeve (links); undefined without an accent. */
  accentInk?: string;
}

/** The accent's colours on the page: amber's three roles. */
export interface AccentColors {
  /** The play control's fill and the equaliser (amber). */
  fill: string;
  /** The glyph on the fill (on-amber). */
  onFill: string;
  /** The accent as text on the ground and on rows (amber-ink). */
  ink: string;
}

export interface AlbumColors {
  sleeve: SleeveColors;
  accent?: AccentColors;
}

/** The backgrounds page text sits on: ground, row hover, pressed, and a selected row (ink at 12%). */
export function pageBackgrounds(theme: ThemeName): string[] {
  const t = THEMES[theme];
  return [t.ground, t.hover, t.active, over(t.ground, t.ink, 0.12)];
}
/**
 * `primary` is Apple's primary text colour; `softer` its secondary and
 * softer ones, in order of preference.
 */
function sleeveColors(
  bg: string,
  primary: string | undefined,
  softer: readonly string[],
  theme: ThemeName,
): SleeveColors {
  const tones = tonesOf(sleeveSurface(bg, theme));
  const [surface, hover, active] = tones;
  const readable = (c: string, min = TEXT_CONTRAST) => worst(c, tones) >= min;
  // Apple's primary when it reads on the sleeve as it is, with room for a
  // muted step under it; else (the sleeve moved off the cover's colour)
  // black or white with a trace of the sleeve.
  const ink =
    primary && readable(primary, TEXT_CONTRAST * MUTED_STEP)
      ? primary
      : ensureContrast(mix(poleOf(surface), surface, 0.03), tones, TEXT_CONTRAST);
  const derived = mix(ink, surface, 0.35);
  const inkMuted =
    [...softer, derived].find((c) => readable(c) && contrast(c, ink) >= MUTED_STEP) ??
    ensureContrast(derived, tones, TEXT_CONTRAST);
  return {
    surface,
    hover,
    active,
    ink,
    inkMuted,
    line: mix(surface, ink, 0.14),
    controlLine: ensureContrast(mix(surface, ink, 0.5), [surface, hover], UI_CONTRAST),
  };
}

function accentColors(accent: string, theme: ThemeName, under: readonly string[]): AccentColors {
  const t = THEMES[theme];
  // The fill (play control, equaliser) is never less distinct from what
  // it sits on than amber is from the ground, nor than 3:1 where amber is.
  const floor = Math.min(UI_CONTRAST, contrast(t.amber, t.ground));
  const glyphs = [t.onAmber, WHITE];
  const glyph = (c: string) => glyphs.reduce((a, b) => (contrast(b, c) > contrast(a, c) ? b : a));
  const fillOk = (c: string) => worst(c, under) >= floor && contrast(glyph(c), c) >= TEXT_CONTRAST;
  const fill = nearest(accent, fillOk) ?? t.amber;
  return {
    fill,
    onFill: fill === t.amber ? t.onAmber : glyph(fill),
    ink: ensureContrast(accent, pageBackgrounds(theme), TEXT_CONTRAST),
  };
}

/**
 * The album's colours in `theme`, from Apple's colours for the cover and
 * Go's dominant colours (`Header.colors`). `playOnSleeve`: the play
 * control sits on the sleeve (the narrow layout), not beside it. Undefined
 * when neither gives a background: the sleeve then keeps the plain wash
 * of its backdrop.
 */
export function albumColors(
  apple: ArtworkColors | undefined,
  dominant: readonly string[],
  theme: ThemeName,
  playOnSleeve = false,
): AlbumColors | undefined {
  const valid = (c: string | undefined): c is string => !!c && !!parseHex(c);
  const bg = [apple?.background, ...dominant].find(valid);
  if (!bg) return undefined;
  // Apple's text colours are picked for its background, so only with it.
  const own = bg === apple?.background ? apple : undefined;
  const sleeve = sleeveColors(
    bg,
    [own?.text1].find(valid),
    [own?.text2, own?.text3].filter(valid),
    theme,
  );
  const hue = accentOf(
    [apple?.background, apple?.text1, apple?.text2, apple?.text3, apple?.text4, ...dominant].filter(
      valid,
    ),
  );
  if (!hue) return { sleeve };
  sleeve.accentInk = ensureContrast(
    hue,
    [sleeve.surface, sleeve.hover, sleeve.active],
    TEXT_CONTRAST,
  );
  const under = playOnSleeve ? [THEMES[theme].ground, sleeve.surface] : [THEMES[theme].ground];
  return { sleeve, accent: accentColors(hue, theme, under) };
}

type Vars = Record<`--${string}`, string>;

/** Custom properties that put the accent in amber's place on the page. */
export function accentVars(a: AccentColors | undefined): Vars {
  if (!a) return {};
  return { "--amber": a.fill, "--on-amber": a.onFill, "--amber-ink": a.ink };
}

/**
 * Custom properties for the sleeve: its tokens, plus the ShadCN roles that
 * `index.css` maps onto tokens (resolved where they are declared, so they
 * are declared again here to follow the sleeve's).
 */
export function sleeveVars(s: SleeveColors): Vars {
  return {
    "--sleeve": s.surface,
    "--ink": s.ink,
    "--ink-muted": s.inkMuted,
    "--ink-faint": s.inkMuted,
    "--hover": s.hover,
    "--active": s.active,
    "--line": s.line,
    "--control-line": s.controlLine,
    ...(s.accentInk ? { "--amber-ink": s.accentInk } : {}),
    "--foreground": s.ink,
    "--primary": s.ink,
    "--primary-foreground": s.surface,
    "--secondary": s.hover,
    "--secondary-foreground": s.ink,
    "--muted": s.hover,
    "--muted-foreground": s.inkMuted,
    "--accent": s.hover,
    "--accent-foreground": s.ink,
    "--border": s.line,
    "--input": s.controlLine,
    "--ring": s.ink,
  };
}
