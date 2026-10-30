import { afterEach, describe, expect, it } from "vitest";

import { mirrorModalInert } from "./modal-inert";

// Mutation observers report in a microtask queued before this one.
const flush = () => Promise.resolve();

describe("mirrorModalInert", () => {
  let stop = () => {};
  afterEach(() => {
    stop();
    document.body.innerHTML = "";
  });

  it("makes what a modal hides inert and restores it when the modal goes", async () => {
    document.body.innerHTML = `<nav><a href="#">Home</a></nav><div id="portal"></div>`;
    stop = mirrorModalInert();
    const nav = document.querySelector("nav")!;

    nav.setAttribute("aria-hidden", "true");
    nav.setAttribute("data-aria-hidden", "true");
    await flush();
    expect(nav.hasAttribute("inert")).toBe(true);

    nav.removeAttribute("aria-hidden");
    nav.removeAttribute("data-aria-hidden");
    await flush();
    expect(nav.hasAttribute("inert")).toBe(false);
  });

  it("leaves an element that was already inert inert", async () => {
    document.body.innerHTML = `<main inert></main>`;
    stop = mirrorModalInert();
    const main = document.querySelector("main")!;

    main.setAttribute("data-aria-hidden", "true");
    await flush();
    main.removeAttribute("data-aria-hidden");
    await flush();
    expect(main.hasAttribute("inert")).toBe(true);
  });
});
