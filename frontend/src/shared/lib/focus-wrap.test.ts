import { afterEach, describe, expect, it } from "vitest";

import { wrapTabFocus } from "./focus-wrap";

function tab(shiftKey = false) {
  const e = new KeyboardEvent("keydown", { key: "Tab", shiftKey, bubbles: true, cancelable: true });
  (document.activeElement ?? document.body).dispatchEvent(e);
  return e;
}

const el = (id: string) => document.getElementById(id)!;

describe("wrapTabFocus", () => {
  let stop = () => {};
  afterEach(() => {
    stop();
    document.body.innerHTML = "";
  });

  it("wraps Tab from the last element to the first, and Shift+Tab back", () => {
    document.body.innerHTML = `<a id="skip" href="#">Skip</a><main tabindex="-1"><button id="mid">Play</button></main><footer><button id="last">Volume</button></footer>`;
    stop = wrapTabFocus();
    el("last").focus();
    const e = tab();
    expect(e.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(el("skip"));
    expect(tab(true).defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(el("last"));
  });

  it("leaves Tab between elements, and from a container into its children, to the browser", () => {
    document.body.innerHTML = `<a id="skip" href="#">Skip</a><main id="main" tabindex="-1"><button id="mid">Play</button></main><button id="last">Volume</button>`;
    stop = wrapTabFocus();
    el("mid").focus();
    expect(tab().defaultPrevented).toBe(false);
    expect(tab(true).defaultPrevented).toBe(false);
    el("main").focus();
    expect(tab().defaultPrevented).toBe(false);
    expect(tab(true).defaultPrevented).toBe(false);
  });

  it("skips what Tab cannot reach when finding the ends", () => {
    document.body.innerHTML = `<button id="first">First</button><button id="last">Last</button><button disabled>Off</button><div inert><button>Behind a dialog</button></div><div hidden><button>Hidden</button></div><span style="display:none"><button>None</button></span><button style="visibility:hidden">Invisible</button><a>No link</a><input type="hidden"><button tabindex="-1">Roving</button>`;
    stop = wrapTabFocus();
    el("last").focus();
    expect(tab().defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(el("first"));
  });

  it("does not wrap a Tab a focus trap already handled", () => {
    document.body.innerHTML = `<button id="first">First</button><button id="last">Last</button>`;
    el("last").addEventListener("keydown", (e) => e.preventDefault());
    stop = wrapTabFocus();
    el("last").focus();
    tab();
    expect(document.activeElement).toBe(el("last"));
  });

  it("sends Shift+Tab with nothing focused to the last element", () => {
    document.body.innerHTML = `<button id="first">First</button><button id="last">Last</button>`;
    stop = wrapTabFocus();
    expect(tab().defaultPrevented).toBe(false);
    tab(true);
    expect(document.activeElement).toBe(el("last"));
  });
});
