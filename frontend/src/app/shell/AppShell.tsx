import { useLingui } from "@lingui/react/macro";
import { Outlet, useMatches } from "@tanstack/react-router";
import { useEffect, type CSSProperties, type ReactNode } from "react";

import { cn } from "@/shared/lib/utils";

import { useGoBridge } from "../bridge";
import { slots } from "../compose";
import { ShellProviders } from "../providers";
import { useShortcuts } from "../shortcuts";
import { useLayout } from "./layout";
import { Sidebar } from "./Sidebar";
import { StatusBanner } from "./StatusBanner";
import { TitleBarStart } from "./TitleBarStart";
import { TopBar } from "./TopBar";
import { useRouteMemory } from "./useRouteMemory";
import { bindWindowState, titleBarRegion, useWindowChrome } from "./window-chrome";
import { WindowControls } from "./WindowControls";

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
 * A full-window page's frame (route staticData.fullWindow, the first-run
 * setup): the title bar's band over the page's top `--title-bar-height`
 * pixels, with the mark at its leading end, as in the sidebar's header,
 * and the window's own buttons where the desktop puts them; nothing else
 * of the shell. The page lays itself out under the band and keeps that
 * strip free of controls, since it drags the window.
 */
function FullWindow() {
  const chrome = useWindowChrome();
  const region = titleBarRegion(chrome);
  const trailing = chrome.end.length > 0;
  return (
    <div
      className="relative h-full"
      style={{ "--title-bar-height": `${chrome.height}px` } as CSSProperties}
    >
      <SkipLink />
      <header className="absolute inset-x-0 top-0 z-10 flex">
        <TitleBarStart chrome={chrome} className="w-[232px]" />
        <div
          {...region}
          style={{
            ...region.style,
            height: chrome.height,
            paddingInlineEnd: trailing && !chrome.captions ? 8 : 0,
          }}
          className="flex flex-1 items-center justify-end select-none"
        >
          {trailing && <WindowControls chrome={chrome} side="end" />}
        </div>
      </header>
      <Outlet />
    </div>
  );
}

/**
 * The window layout: sidebar and top bar on the surface tone, the routed page
 * on the ground, module asides (the queue) beside it, module bars (now
 * playing) across the bottom. Only the page area scrolls, so it is the
 * scroll element for virtualised lists. Module overlays cover the page column.
 * The sidebar can be hidden (shell.sidebar, Ctrl+B); its column collapses.
 * A full-window page gets only the title bar (FullWindow).
 */
export function AppShell() {
  useGoBridge();
  useShortcuts();
  useRouteMemory();
  useEffect(() => bindWindowState(), []);
  const overlayOpen = useOverlayOpen();
  const sidebar = useLayout((s) => s.sidebar);
  const fullWindow = useMatches({ select: (ms) => ms.some((m) => m.staticData.fullWindow) });

  if (fullWindow) {
    return (
      <ShellProviders>
        <ModuleProviders>
          <FullWindow />
        </ModuleProviders>
      </ShellProviders>
    );
  }
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
      </ModuleProviders>
    </ShellProviders>
  );
}
