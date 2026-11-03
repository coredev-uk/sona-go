import { useLingui } from "@lingui/react/macro";
import { Outlet } from "@tanstack/react-router";
import { useEffect, type ReactNode } from "react";

import { cn } from "@/shared/lib/utils";

import { useGoBridge } from "../bridge";
import { slots } from "../compose";
import { ShellProviders } from "../providers";
import { useShortcuts } from "../shortcuts";
import { useLayout } from "./layout";
import { Sidebar } from "./Sidebar";
import { StatusBanner } from "./StatusBanner";
import { TopBar } from "./TopBar";
import { useDeveloperContextMenu } from "./useDeveloperContextMenu";
import { useMouseNavigation } from "./useMouseNavigation";
import { useRouteMemory } from "./useRouteMemory";
import { bindWindowState } from "./window-chrome";

const providers = slots.flatMap((s) => (s.Provider ? [s.Provider] : []));
const asides = slots.flatMap((s) => (s.Aside ? [s.Aside] : []));
const overlays = slots.flatMap((s) => (s.overlay ? [s.overlay] : []));
const bottoms = slots.flatMap((s) => (s.Bottom ? [s.Bottom] : []));
const layers = slots.flatMap((s) => (s.Layer ? [s.Layer] : []));

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
 * The first Tab stop: jumps past the sidebar to the page. The router owns
 * the URL hash, so it focuses <main> instead of following `#content`.
 */
function SkipLink() {
  const { t } = useLingui();
  return (
    <a
      href="#content"
      onClick={(e) => {
        e.preventDefault();
        document.getElementById("content")?.focus();
      }}
      className="fixed top-2 left-2 z-[60] -translate-y-16 rounded-md bg-raised px-3 py-2 text-ui font-medium text-ink shadow-float focus:translate-y-0"
    >
      {t`Skip to content`}
    </a>
  );
}

/**
 * The window layout: sidebar and top bar on the surface tone, the routed page
 * on the ground, module asides (the queue) beside it, module bars (now
 * playing) across the bottom. Only the page area scrolls, so it is the
 * scroll element for virtualised lists. Module overlays cover the page column;
 * module layers (the first-run setup) cover the whole window.
 * The sidebar can be hidden (shell.sidebar, Ctrl+B); its column collapses.
 */
export function AppShell() {
  useGoBridge();
  useShortcuts();
  useRouteMemory();
  useMouseNavigation();
  useDeveloperContextMenu();
  useEffect(() => bindWindowState(), []);
  const overlayOpen = useOverlayOpen();
  const sidebar = useLayout((s) => s.sidebar);

  return (
    <ShellProviders>
      <ModuleProviders>
        <div
          className={cn(
            "grid h-full grid-rows-[minmax(0,1fr)_auto]",
            sidebar ? "grid-cols-[232px_minmax(0,1fr)]" : "grid-cols-[0px_minmax(0,1fr)]",
          )}
        >
          <SkipLink />
          {sidebar && <Sidebar />}
          <div
            className={cn(
              "col-start-2 row-start-1 flex min-h-0 min-w-0 flex-col",
              sidebar && "border-l border-line",
            )}
          >
            <TopBar />
            <StatusBanner />
            <div className="relative flex min-h-0 flex-1 border-t border-line">
              <main
                id="content"
                // Focus target for the skip link (not a Tab stop itself).
                tabIndex={-1}
                inert={overlayOpen}
                className="min-h-0 min-w-0 flex-1 overflow-y-auto bg-ground focus-visible:outline-offset-[-2px]"
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
        {layers.map((Layer, i) => (
          <Layer key={i} />
        ))}
      </ModuleProviders>
    </ShellProviders>
  );
}
