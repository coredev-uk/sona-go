import { afterAll } from "vitest";

/**
 * Test setup (`setupFiles` in vite.config.ts): once a test file's tests are
 * done, clear every timer it started. Vitest then tears the file's jsdom
 * window down, so a timer still pending would fire without a `window` and
 * fail the run with `ReferenceError: window is not defined`. The Wails
 * runtime, loaded through the bindings, polls `window` like that for its drag
 * handlers right after it loads.
 */
const started: ReturnType<typeof setTimeout>[] = [];
const { setTimeout: realSetTimeout, setInterval: realSetInterval } = globalThis;

globalThis.setTimeout = ((...args: Parameters<typeof realSetTimeout>) => {
  const id = realSetTimeout(...args);
  started.push(id);
  return id;
}) as typeof realSetTimeout;

globalThis.setInterval = ((...args: Parameters<typeof realSetInterval>) => {
  const id = realSetInterval(...args);
  started.push(id);
  return id;
}) as typeof realSetInterval;

afterAll(() => {
  // clearTimeout clears intervals too (one list of timers in Node and HTML).
  for (const id of started.splice(0)) clearTimeout(id);
});
