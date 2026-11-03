import type { ArtworkColors } from "@bindings/services";
import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

import {
  accentOf,
  albumColors,
  contrast,
  ensureContrast,
  lightness,
  pageBackgrounds,
  sleeveSurface,
  sleeveVars,
  TEXT_CONTRAST,
  THEMES,
  UI_CONTRAST,
  type ThemeName,
} from "./palette";

const themes: ThemeName[] = ["light", "dark"];

function apple(background: string, ...text: string[]): ArtworkColors {
  return {
    background,
    text1: text[0] ?? "",
    text2: text[1] ?? "",
    text3: text[2] ?? "",
    text4: text[3] ?? "",
  };
}

/** Covers at the edges of what Apple sends, with the text colours Apple gave or none. */
const covers: Record<string, ArtworkColors> = {
  white: apple("#ffffff", "#0c0c0c", "#191919", "#3d3d3d", "#474747"),
  black: apple("#040a0a", "#dfebe5", "#dfebe5", "#b3beb9", "#b3beb9"),
  midGrey: apple("#777777"),
  cream: apple("#fffce9", "#050608", "#232429"),
  lime: apple("#8acf00", "#050800", "#070a00", "#203000", "#213100"),
  navyGold: apple("#3f546a", "#fdc14c", "#fcb92d", "#d7ab52", "#d6a539"),
  red: apple("#ff0000"),
  green: apple("#00ff00"),
  blue: apple("#0000ff", "#ffffff"),
  yellow: apple("#ffff00", "#ffff00", "#ffff00"),
  cyan: apple("#00ffff"),
  magenta: apple("#ff00ff", "#00ff00", "#ff00ff"),
  // Apple's text colours unreadable on its own background.
  clash: apple("#808080", "#7f7f7f", "#818181"),
  ground: apple(THEMES.light.ground),
  darkGround: apple(THEMES.dark.ground),
};

describe("albumColors", () => {
  for (const theme of themes) {
    for (const [name, colors] of Object.entries(covers)) {
      it(`keeps every pair readable: ${name}, ${theme}`, () => {
        const a = albumColors(colors, [], theme);
        expect(a).toBeDefined();
        const { sleeve, accent } = a!;
        const tones = [sleeve.surface, sleeve.hover, sleeve.active];
        for (const bg of tones) {
          expect(contrast(sleeve.ink, bg)).toBeGreaterThanOrEqual(TEXT_CONTRAST);
          expect(contrast(sleeve.inkMuted, bg)).toBeGreaterThanOrEqual(TEXT_CONTRAST);
          if (sleeve.accentInk) {
            expect(contrast(sleeve.accentInk, bg)).toBeGreaterThanOrEqual(TEXT_CONTRAST);
          }
        }
        for (const bg of [sleeve.surface, sleeve.hover]) {
          expect(contrast(sleeve.controlLine, bg)).toBeGreaterThanOrEqual(UI_CONTRAST);
        }
        // One tone step off the ground; never a bright slab in the dark theme.
        const ground = lightness(THEMES[theme].ground);
        expect(Math.abs(lightness(sleeve.surface) - ground)).toBeGreaterThanOrEqual(0.045);
        if (theme === "dark") expect(lightness(sleeve.surface)).toBeLessThanOrEqual(0.37);
        if (accent) {
          const t = THEMES[theme];
          expect(contrast(accent.onFill, accent.fill)).toBeGreaterThanOrEqual(TEXT_CONTRAST);
          const floor = Math.min(UI_CONTRAST, contrast(t.amber, t.ground));
          expect(contrast(accent.fill, t.ground)).toBeGreaterThanOrEqual(floor - 1e-9);
          for (const bg of pageBackgrounds(theme)) {
            expect(contrast(accent.ink, bg)).toBeGreaterThanOrEqual(TEXT_CONTRAST);
          }
          // In the narrow layout the play control sits on the sleeve.
          const narrow = albumColors(colors, [], theme, true)!.accent!;
          expect(contrast(narrow.onFill, narrow.fill)).toBeGreaterThanOrEqual(TEXT_CONTRAST);
          if (narrow.fill !== t.amber) {
            expect(contrast(narrow.fill, sleeve.surface)).toBeGreaterThanOrEqual(floor - 1e-9);
            expect(contrast(narrow.fill, t.ground)).toBeGreaterThanOrEqual(floor - 1e-9);
          }
        }
      });
    }
  }

  it("uses Apple's own colours when they already pass, its softer one for muted text", () => {
    const { sleeve } = albumColors(covers.lime, [], "light")!;
    expect(sleeve.surface).toBe("#8acf00");
    expect(sleeve.ink).toBe("#050800");
    // Apple's secondary is the same colour as its primary here; its softer one reads as a step down.
    expect(sleeve.inkMuted).toBe("#203000");
  });

  it("keeps muted text a step below the ink", () => {
    for (const theme of themes) {
      for (const colors of Object.values(covers)) {
        const { sleeve } = albumColors(colors, [], theme)!;
        expect(contrast(sleeve.ink, sleeve.inkMuted)).toBeGreaterThan(1.1);
      }
    }
  });

  it("turns the ink to the other side when the sleeve leaves the cover's colour", () => {
    // In the dark theme the cream sleeve darkens, so Apple's near-black text can't stay.
    const { sleeve } = albumColors(covers.cream, [], "dark")!;
    expect(lightness(sleeve.ink)).toBeGreaterThan(0.8);
    expect(contrast(sleeve.ink, sleeve.active)).toBeGreaterThanOrEqual(TEXT_CONTRAST);
  });

  // The sweep runs albumColors 432 times, ~0.6 s on an idle machine; a loaded CI
  // runner takes several times that, so the budget is 30 s rather than vitest's 5 s.
  it(
    "keeps every pair readable for every cover colour on a 6-level grid, with any text colours",
    { timeout: 30_000 },
    () => {
      const levels = ["00", "33", "66", "99", "cc", "ff"];
      const grid = levels.flatMap((r) => levels.flatMap((g) => levels.map((b) => `#${r}${g}${b}`)));
      let failures = 0;
      for (const theme of themes) {
        grid.forEach((bg, i) => {
          // Apple's text colours: none, or arbitrary ones from the grid.
          const text =
            i % 3 === 0 ? [] : [grid[(i * 7) % grid.length], grid[(i * 13) % grid.length]];
          const { sleeve, accent } = albumColors(
            apple(bg, ...text),
            [grid[(i * 5) % grid.length]],
            theme,
          )!;
          const tones = [sleeve.surface, sleeve.hover, sleeve.active];
          const texts = [
            sleeve.ink,
            sleeve.inkMuted,
            ...(sleeve.accentInk ? [sleeve.accentInk] : []),
          ];
          if (texts.some((c) => tones.some((t) => contrast(c, t) < TEXT_CONTRAST))) failures++;
          if (accent) {
            if (contrast(accent.onFill, accent.fill) < TEXT_CONTRAST) failures++;
            if (pageBackgrounds(theme).some((b) => contrast(accent.ink, b) < TEXT_CONTRAST))
              failures++;
          }
        });
      }
      expect(failures).toBe(0);
    },
  );

  it("takes amber's job only for a cover with colour", () => {
    expect(albumColors(covers.white, [], "light")!.accent).toBeUndefined();
    expect(albumColors(covers.black, [], "dark")!.accent).toBeUndefined();
    expect(albumColors(covers.lime, [], "light")!.accent).toBeDefined();
    // Gold text on navy: the most colourful gold is the accent.
    const gold = albumColors(covers.navyGold, [], "dark")!.accent!;
    expect(gold.ink).toBe("#fcb92d");
  });

  it("falls back to Go's dominant colours, then to nothing", () => {
    const a = albumColors(undefined, ["#3366cc", "#ffcc00"], "light")!;
    // The first dominant colour, darkened so white text and a muted step under it pass on its pressed tone too.
    expect(contrast(a.sleeve.surface, "#3366cc")).toBeLessThan(1.5);
    expect(lightness(a.sleeve.surface)).toBeLessThan(lightness("#3366cc"));
    expect(contrast(a.sleeve.ink, a.sleeve.active)).toBeGreaterThanOrEqual(TEXT_CONTRAST);
    expect(albumColors(apple(""), ["not a colour"], "light")).toBeUndefined();
    expect(albumColors(undefined, [], "dark")).toBeUndefined();
  });
});

describe("sleeveSurface", () => {
  it("steps a cover the colour of the ground off it, the way the theme's tones go", () => {
    expect(lightness(sleeveSurface(THEMES.light.ground, "light"))).toBeLessThan(
      lightness(THEMES.light.ground),
    );
    expect(lightness(sleeveSurface(THEMES.dark.ground, "dark"))).toBeGreaterThan(
      lightness(THEMES.dark.ground),
    );
  });
});

describe("ensureContrast", () => {
  it("returns the colour itself when it passes", () => {
    expect(ensureContrast("#000000", ["#ffffff"], 4.5)).toBe("#000000");
  });

  it("finds a passing colour against a mid grey, the hardest background", () => {
    for (const fg of ["#7f7f7f", "#ff0000", "#00ff00", "#0000ff"]) {
      const c = ensureContrast(fg, ["#767676", "#777777", "#787878"], 4.5);
      expect(contrast(c, "#777777")).toBeGreaterThanOrEqual(4.5);
    }
  });
});

describe("accentOf", () => {
  it("picks the most colourful candidate and ignores greys and junk", () => {
    expect(accentOf(["#777777", "#ff0000", "#cc4444", "x"])).toBe("#ff0000");
    expect(accentOf(["#ffffff", "#0c0c0c", "#191919"])).toBeUndefined();
  });
});

describe("sleeveVars", () => {
  it("declares the ShadCN roles again so stock controls follow the sleeve", () => {
    const s = albumColors(covers.lime, [], "dark")!.sleeve;
    const vars = sleeveVars(s);
    expect(vars["--input"]).toBe(s.controlLine);
    expect(vars["--ring"]).toBe(s.ink);
    expect(vars["--accent"]).toBe(s.hover);
  });
});

it("THEMES matches the tokens in index.css", () => {
  const css = readFileSync(
    path.resolve(import.meta.dirname, "../../shared/styles/index.css"),
    "utf8",
  );
  const block = (selector: RegExp) => {
    const m = selector.exec(css);
    expect(m).not.toBeNull();
    return css.slice(m!.index, css.indexOf("}", m!.index));
  };
  const token = (body: string, name: string) =>
    new RegExp(`--${name}:\\s*(#[0-9a-f]{6})`, "i").exec(body)?.[1]?.toLowerCase();
  const names = {
    ground: "ground",
    hover: "hover",
    active: "active",
    ink: "ink",
    amber: "amber",
    onAmber: "on-amber",
  };
  for (const [theme, selector] of [
    ["light", /:root,\s*\.theme-light\s*\{/],
    ["dark", /\n\.dark\s*\{/],
  ] as const) {
    const body = block(selector);
    for (const [key, css] of Object.entries(names)) {
      expect(token(body, css), `${theme} ${css}`).toBe(THEMES[theme][key as keyof typeof names]);
    }
  }
});
