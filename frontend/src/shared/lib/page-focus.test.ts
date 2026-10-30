import { afterEach, describe, expect, it } from "vitest";

import { returnTarget, trackPageFocus } from "./page-focus";

describe("returnTarget", () => {
  let stop = () => {};
  afterEach(() => {
    stop();
    document.body.innerHTML = "";
  });

  it("returns to the opener while it is still on the page", () => {
    document.body.innerHTML = `<button id="open">Open</button>`;
    stop = trackPageFocus();
    const open = document.getElementById("open")!;
    expect(returnTarget(open)).toBe(open);
  });

  it("returns to the page element focused before a menu whose item opened the dialog", () => {
    document.body.innerHTML = `<div id="row" tabindex="0"></div><div role="menu"><div id="item" role="menuitem" tabindex="-1"></div></div>`;
    stop = trackPageFocus();
    const row = document.getElementById("row")!;
    const item = document.getElementById("item")!;
    row.focus();
    item.focus();
    item.closest("[role=menu]")!.remove();
    expect(returnTarget(item)).toBe(row);
  });

  it("returns to a menu's trigger when the button focused before it was remounted", () => {
    document.body.innerHTML = `<button id="old">More</button><button id="trigger" aria-controls="m">More</button><div id="m" role="menu"><div id="item" role="menuitem" tabindex="-1"></div></div>`;
    stop = trackPageFocus();
    document.getElementById("old")!.focus();
    document.getElementById("old")!.remove();
    const item = document.getElementById("item")!;
    item.focus();
    document.getElementById("m")!.remove();
    expect(returnTarget(item)).toBe(document.getElementById("trigger"));
  });

  it("gives up when the remembered element left the page too", () => {
    document.body.innerHTML = `<div id="row" tabindex="0"></div>`;
    stop = trackPageFocus();
    const row = document.getElementById("row")!;
    row.focus();
    row.remove();
    expect(returnTarget(null)).toBeNull();
  });
});
