import { Outlet } from "@tanstack/react-router";
import type { ReactNode } from "react";

import { useGoBridge } from "../bridge";
import { shortcuts, slots } from "../compose";
import { ShellProviders } from "../providers";
import { useShortcuts } from "../shortcuts";
import { Sidebar } from "./Sidebar";
import { StatusBanner } from "./StatusBanner";
import { TopBar } from "./TopBar";

const providers = slots.flatMap((s) => (s.Provider ? [s.Provider] : []));
const asides = slots.flatMap((s) => (s.Aside ? [s.Aside] : []));
const overlays = slots.flatMap((s) => (s.overlay ? [s.overlay] : []));
const bottoms = slots.flatMap((s) => (s.Bottom ? [s.Bottom] : []));

/** True while any module's overlay covers the page. */
function useOverlayOpen(): boolean {
  let open = false;
  // The slot list is fixed when the app loads, so the hooks run in the same order every render.
  for (const o of overlays) open = o.useOpen() || open;
  return open;
}

function ModuleProviders({ children }: { children: ReactNode }) {
  return providers.reduceRight<ReactNode>(
    (inner, Provider) => <Provider>{inner}</Provider>,
    children,
  );
}

/**
 * The window layout: sidebar and top bar on the surface tone, the routed page
 * on the ground, module asides (the queue) beside it, module bars (now
 * playing) across the bottom. Only the page area scrolls, so it is the
 * scroll element for virtualised lists. Module overlays cover the page column.
 */
export function AppShell() {
  useGoBridge();
  useShortcuts(shortcuts);
  const overlayOpen = useOverlayOpen();

  return (
    <ShellProviders>
      <ModuleProviders>
        <div className="grid h-full grid-cols-[232px_minmax(0,1fr)] grid-rows-[minmax(0,1fr)_auto]">
          <Sidebar />
          <div className="flex min-h-0 min-w-0 flex-col border-l border-line">
            <TopBar />
            <StatusBanner />
            <div className="relative flex min-h-0 flex-1 border-t border-line">
              <main
                id="content"
                inert={overlayOpen}
                className="min-h-0 min-w-0 flex-1 overflow-y-auto bg-ground"
              >
                <div className="px-8 pt-7 pb-12 max-[960px]:px-6">
                  <Outlet />
                </div>
              </main>
              {asides.map((Aside, i) => (
                <Aside key={i} />
              ))}
              {overlays.map(({ Component }, i) => (
                <Component key={i} />
              ))}
            </div>
          </div>
          {bottoms.map((Bottom, i) => (
            <Bottom key={i} />
          ))}
        </div>
      </ModuleProviders>
    </ShellProviders>
  );
}
