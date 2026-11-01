// Loads this page's copy of the runtime first: it sets dispatchWailsEvent.
import "@wailsio/runtime";

/** Delivers one event from Go to a copy of the runtime's listeners. */
type Dispatch = (event: { name: string; data?: unknown; sender?: string }) => void;

declare global {
  interface Window {
    /** The Wails runtime's page globals; Go calls `dispatchWailsEvent`. */
    _wails?: { dispatchWailsEvent?: Dispatch };
  }
}

/**
 * Keeps Go's events reaching this page's copy of the Wails runtime when
 * another copy loads into the page.
 *
 * Go delivers every event by calling `window._wails.dispatchWailsEvent`,
 * and each copy of `@wailsio/runtime` sets that to its own dispatcher, with
 * its own listeners, as it loads. Importing `/wails/runtime.js` into a
 * running page (the Wails MCP server's `call_bound_method` does, and its
 * `js_eval` suggests it) loads a second copy, which then took every later
 * event away from the app's handlers: a library sync's progress and
 * `sync:done`, playback state, settings. Here the property becomes a
 * fan-out instead: the page's dispatcher keeps running first, then each
 * later copy's, in the order they loaded.
 *
 * Call once at startup.
 */
export function shareEventDispatch(target: Pick<Window, "_wails"> = window): void {
  const wails = target._wails;
  const own = wails?.dispatchWailsEvent;
  if (!wails || !own) return;
  const dispatchers: Dispatch[] = [own];
  const dispatch: Dispatch = (event) => {
    for (const d of dispatchers) d(event);
  };
  Object.defineProperty(wails, "dispatchWailsEvent", {
    configurable: true,
    enumerable: true,
    get: () => dispatch,
    set: (d: unknown) => {
      if (typeof d === "function" && d !== dispatch && !dispatchers.includes(d as Dispatch)) {
        dispatchers.push(d as Dispatch);
      }
    },
  });
}
