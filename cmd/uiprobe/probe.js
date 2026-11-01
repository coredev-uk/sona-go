// UI performance probe, installed into a running -tags mcp Sona through the
// Wails MCP server's js_eval. Defines window.__sonaProbe.
(() => {
  if (window.__sonaProbe) return "installed";
  const dpr = () => Math.min(2, Math.max(1, devicePixelRatio || 1));
  const loads = [];
  document.addEventListener(
    "load",
    (e) => {
      const img = e.target;
      if (!(img instanceof HTMLImageElement) || !img.src.includes("/art/")) return;
      const r = img.getBoundingClientRect();
      loads.push({
        src: img.currentSrc || img.src,
        nw: img.naturalWidth,
        nh: img.naturalHeight,
        bw: r.width,
        bh: r.height,
        t: performance.now(),
      });
    },
    true,
  );
  const scroller = () => document.getElementById("content");
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const raf = () => new Promise((r) => requestAnimationFrame(r));

  // Waits until every on-screen image is complete and the DOM stops growing.
  async function settle(timeout = 20000) {
    const end = performance.now() + timeout;
    let last = -1;
    let stable = 0;
    while (performance.now() < end) {
      const imgs = [...document.images].filter((i) => {
        const b = i.getBoundingClientRect();
        return b.bottom > 0 && b.top < innerHeight && b.width > 0;
      });
      const n = document.getElementsByTagName("*").length;
      const done = imgs.every((i) => i.complete) && n === last;
      stable = done ? stable + 1 : 0;
      if (stable >= 4) return true;
      last = n;
      await sleep(150);
    }
    return false;
  }

  function quantiles(xs) {
    const s = [...xs].sort((a, b) => a - b);
    const q = (p) => (s.length ? s[Math.min(s.length - 1, Math.floor(p * s.length))] : 0);
    return { p50: q(0.5), p95: q(0.95), p99: q(0.99), max: s[s.length - 1] ?? 0 };
  }

  // Frame deltas for `ms` with nothing happening: the display's frame interval.
  async function idle(ms = 2000) {
    const ds = [];
    let prev = await raf();
    const end = prev + ms;
    for (;;) {
      const t = await raf();
      ds.push(t - prev);
      prev = t;
      if (t > end) break;
    }
    return quantiles(ds);
  }

  function imageStats(since) {
    const d = dpr();
    const ls = loads.filter((l) => l.t >= since);
    const seen = new Set();
    let px = 0;
    let shown = 0;
    let over2 = 0;
    for (const l of ls) {
      if (seen.has(l.src)) continue;
      seen.add(l.src);
      px += l.nw * l.nh;
      const box = l.bw * l.bh * d * d;
      shown += box;
      if (box > 0 && l.nw * l.nh > box * 1.5) over2++;
    }
    return {
      loads: ls.length,
      distinct: seen.size,
      decodedMP: +(px / 1e6).toFixed(1),
      // Decoded pixels per displayed device pixel (1 = exact).
      ratio: shown ? +(px / shown).toFixed(2) : 0,
      oversized: over2,
    };
  }

  let interval = 16.67;

  /** Scrolls #content to the top, waits for its images, and reads the frame interval. */
  async function top() {
    scroller().scrollTop = 0;
    await sleep(300);
    await settle(10000);
    interval = (await idle(500)).p50;
    return interval;
  }

  /**
   * Scrolls #content from the top to the bottom (at most `maxPx`) at
   * `speed` CSS px/s, one step per frame, and back to the top, recording
   * every frame delta. Call top() first.
   */
  async function scroll({ speed = 2000, maxPx = 20000, back = true, hover = false } = {}) {
    const el = scroller();
    const t0 = performance.now();
    const ds = [];
    // hover: a pointer resting over the middle of #content while it scrolls,
    // as a mouse wheel leaves it: each element passing under it is hovered.
    const box = el.getBoundingClientRect();
    const hx = box.left + box.width / 2;
    const hy = box.top + Math.min(box.height, innerHeight) / 2;
    let under = null;
    const point = () => {
      const next = document.elementFromPoint(hx, hy);
      if (next === under) return;
      under?.dispatchEvent(new PointerEvent("pointerout", { bubbles: true, relatedTarget: next }));
      next?.dispatchEvent(new PointerEvent("pointerover", { bubbles: true, relatedTarget: under }));
      under = next;
    };
    let videos = 0;
    const run = async (from, sign) => {
      let prev = await raf();
      const start = prev;
      for (;;) {
        const t = await raf();
        ds.push(t - prev);
        prev = t;
        const limit = Math.min(maxPx, el.scrollHeight - el.clientHeight);
        const y = from + sign * (speed * (t - start)) / 1000;
        el.scrollTop = Math.max(0, Math.min(limit, y));
        if (hover) point();
        videos = Math.max(videos, document.querySelectorAll("video").length);
        if (sign > 0 ? y >= limit : y <= 0) break;
      }
    };
    await run(0, 1);
    const bottom = el.scrollTop;
    if (back) await run(bottom, -1);
    if (hover) unhover();
    const dur = performance.now() - t0;
    const budget = interval * 1.5;
    let dropped = 0;
    let long = 0;
    for (const d of ds) {
      if (d > budget) dropped += Math.round(d / interval) - 1;
      if (d > 50) long++;
    }
    return {
      interval: +interval.toFixed(2),
      px: Math.round(bottom),
      frames: ds.length,
      ms: Math.round(dur),
      ...Object.fromEntries(Object.entries(quantiles(ds)).map(([k, v]) => [k, +v.toFixed(1)])),
      dropped,
      droppedPct: +((100 * dropped) / (ds.length + dropped)).toFixed(1),
      longFrames: long,
      images: imageStats(t0),
      nodes: document.getElementsByTagName("*").length,
      videos,
    };
  }

  // Elements whose style makes WebKit composite them on their own layer
  // (an estimate: WebKit does not expose its layer tree to the page).
  function layers() {
    const out = {};
    const add = (k) => (out[k] = (out[k] || 0) + 1);
    for (const el of document.querySelectorAll("*")) {
      const s = getComputedStyle(el);
      if (el.tagName === "VIDEO" || el.tagName === "CANVAS") add(el.tagName.toLowerCase());
      if (s.willChange && s.willChange !== "auto") add("will-change");
      if (s.transform && s.transform !== "none" && s.transform.startsWith("matrix3d")) add("3d-transform");
      if (s.backdropFilter && s.backdropFilter !== "none") add("backdrop-filter");
      if (s.webkitBackdropFilter && s.webkitBackdropFilter !== "none") add("backdrop-filter");
      if (s.filter && s.filter !== "none") add("filter");
      if (s.position === "fixed" || s.position === "sticky") add(s.position);
      if (s.animationName && s.animationName !== "none") add("animation");
    }
    return out;
  }

  function page() {
    const d = dpr();
    const imgs = [...document.images].filter((i) => i.src.includes("/art/") && i.naturalWidth);
    let px = 0;
    let box = 0;
    for (const i of imgs) {
      const r = i.getBoundingClientRect();
      px += i.naturalWidth * i.naturalHeight;
      box += r.width * r.height * d * d;
    }
    return {
      nodes: document.getElementsByTagName("*").length,
      imgs: imgs.length,
      ratio: box ? +(px / box).toFixed(2) : 0,
      height: scroller()?.scrollHeight ?? 0,
    };
  }

  // The page's load, in ms since its navigation started (timeOrigin, Unix ms).
  function startup() {
    const n = performance.getEntriesByType("navigation")[0];
    const fcp = performance.getEntriesByName("first-contentful-paint")[0];
    return {
      timeOrigin: performance.timeOrigin,
      responseEnd: Math.round(n?.responseEnd ?? 0),
      domContentLoaded: Math.round(n?.domContentLoadedEventEnd ?? 0),
      fcp: Math.round(fcp?.startTime ?? 0),
    };
  }

  /**
   * Opens a route and waits for it to settle. The route "" is the control:
   * an unknown route (a short not-found page) followed by an empty block
   * 20000 px tall, so a scroll there costs only what scrolling costs.
   */
  async function go(route) {
    document.getElementById("uiprobe-spacer")?.remove();
    location.hash = route === "" ? "#/uiprobe-empty" : `#${route}`;
    await sleep(500);
    if (route === "") {
      const spacer = document.createElement("div");
      spacer.id = "uiprobe-spacer";
      spacer.style.height = "20000px";
      scroller().append(spacer);
    }
    await settle(20000);
    return page();
  }

  // Motion artwork (uiprobe -motion).

  /**
   * Turns Settings → Appearance → Motion artwork on or off with its switch
   * (importing /wails/runtime.js here instead would take Go's events away
   * from the app's own copy of the runtime). Returns the switch's state
   * before.
   */
  async function setMotion(on) {
    location.hash = "#/settings?section=appearance";
    let sw = null;
    for (let i = 0; i < 100 && !(sw = document.getElementById("motion-artwork")); i++) await sleep(100);
    if (!sw) throw new Error("no Motion artwork switch in Settings → Appearance");
    const checked = () => sw.getAttribute("aria-checked") === "true";
    const was = checked();
    if (was !== on) sw.click();
    for (let i = 0; i < 50 && checked() !== on; i++) await sleep(100);
    if (checked() !== on) throw new Error(`the Motion artwork switch stayed ${was ? "on" : "off"}`);
    await sleep(500);
    return was;
  }

  let hovered = null;
  function unhover() {
    hovered?.dispatchEvent(new PointerEvent("pointerout", { bubbles: true, relatedTarget: null }));
    hovered = null;
  }

  /**
   * Hovers the page's index-th cover tile, scrolled into view, as a pointer
   * resting on it does (shared/lib/motion's delegated listeners). index < 0
   * picks the first tile with motion artwork and returns its index, for the
   * motion-off run.
   */
  async function hoverTile(index) {
    unhover();
    const tiles = [...document.querySelectorAll("[data-cover-tile]")];
    if (index < 0) index = tiles.findIndex((t) => t.querySelector("[data-tile-motion]"));
    const tile = tiles[index];
    if (!tile) return -1;
    tile.scrollIntoView({ block: "center", inline: "nearest" });
    await sleep(300);
    tile.dispatchEvent(new PointerEvent("pointerover", { bubbles: true, relatedTarget: null }));
    hovered = tile;
    return index;
  }

  /** Waits until a <video> plays (motion on) or `ms` pass; returns how many play. */
  async function playing(ms = 15000) {
    const end = performance.now() + ms;
    while (performance.now() < end) {
      const n = [...document.querySelectorAll("video")].filter((v) => !v.paused && v.currentTime > 0).length;
      if (n) return n;
      await sleep(100);
    }
    return 0;
  }

  /** The page's motion videos: decoded and drawn size, frames decoded and dropped so far. */
  function videos() {
    return [...document.querySelectorAll("video")].map((v) => {
      const q = v.getVideoPlaybackQuality?.();
      const b = v.getBoundingClientRect();
      return {
        width: v.videoWidth,
        height: v.videoHeight,
        boxWidth: Math.round(b.width * dpr()),
        boxHeight: Math.round(b.height * dpr()),
        paused: v.paused,
        duration: v.duration || 0,
        frames: q?.totalVideoFrames ?? 0,
        dropped: q?.droppedVideoFrames ?? 0,
      };
    });
  }

  /** rAF frame deltas over `ms`, with the share of frames dropped against `interval`. */
  async function frames(ms = 3000) {
    const ds = [];
    let prev = await raf();
    const end = prev + ms;
    for (;;) {
      const t = await raf();
      ds.push(t - prev);
      prev = t;
      if (t > end) break;
    }
    let dropped = 0;
    for (const d of ds) if (d > interval * 1.5) dropped += Math.round(d / interval) - 1;
    const q = quantiles(ds);
    return {
      interval: +interval.toFixed(2),
      p50: +q.p50.toFixed(1),
      p95: +q.p95.toFixed(1),
      frames: ds.length,
      dropped,
    };
  }

  /** The player bar's button that opens Now Playing ("Nothing playing" while idle). */
  const nowPlayingButton = () =>
    document.querySelector('[aria-label="Player"] button[aria-label="Open now playing"]');

  /**
   * Gets a song into the player, paused: plays the album's first song (as
   * its preview with SONA_PREVIEW=1) and pauses it. Returns whether the
   * player has one.
   */
  async function loadSong(album) {
    if (nowPlayingButton()) return true;
    await go(`/album/${album}`);
    // The header's "Play <album>" comes before the songs' "Play <song>".
    const play = [...document.querySelectorAll("main button[aria-label]")].find((b) =>
      b.getAttribute("aria-label").startsWith("Play "),
    );
    if (!play) return false;
    play.click();
    for (let i = 0; i < 100 && !nowPlayingButton(); i++) await sleep(100);
    await sleep(1500);
    document.querySelector('[aria-label="Player"] button[aria-label="Pause"]')?.click();
    await sleep(500);
    return !!nowPlayingButton();
  }

  /** Opens or closes the Now Playing view with the player bar's button. */
  async function nowPlaying(open) {
    const shown = () => !!document.querySelector('section[aria-label="Now playing"]');
    if (shown() !== open) nowPlayingButton()?.click();
    for (let i = 0; i < 50 && shown() !== open; i++) await sleep(100);
    await sleep(500);
    return shown();
  }

  window.__sonaProbe = {
    settle,
    idle,
    top,
    scroll,
    layers,
    page,
    startup,
    go,
    setMotion,
    hoverTile,
    unhover,
    playing,
    videos,
    frames,
    loadSong,
    nowPlaying,
  };
  return "installed";
})();
