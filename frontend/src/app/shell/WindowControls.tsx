import { DesktopService } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { Copy, Minus, Square, X, type LucideIcon } from "lucide-react";
import type { MouseEvent, ReactNode } from "react";

import { cn } from "@/shared/lib/utils";
import { run } from "@/shared/lib/run";

import {
  useWindowActive,
  useWindowStateStore,
  type WindowButton,
  type WindowChrome,
} from "./window-chrome";

/** Windows 11's caption glyphs (Segoe Fluent's ChromeMinimize, …), drawn at 10px. */
function Caption({ children }: { children: ReactNode }) {
  return (
    <svg
      aria-hidden
      viewBox="0 0 10 10"
      className="size-2.5"
      fill="none"
      stroke="currentColor"
      strokeWidth={1}
      shapeRendering="crispEdges"
    >
      {children}
    </svg>
  );
}

const captionGlyph: Record<WindowButton | "restore", ReactNode> = {
  minimize: (
    <Caption>
      <path d="M0 5.5h10" />
    </Caption>
  ),
  maximize: (
    <Caption>
      <rect x="0.5" y="0.5" width="9" height="9" />
    </Caption>
  ),
  restore: (
    <Caption>
      <path d="M2.5 2.5V0.5h7v7h-2" />
      <rect x="0.5" y="2.5" width="7" height="7" />
    </Caption>
  ),
  close: (
    <Caption>
      <path d="M0.5 0.5l9 9M9.5 0.5l-9 9" shapeRendering="geometricPrecision" />
    </Caption>
  ),
};

/** Sona's own glyphs (the top bar's outline icons) for Linux. */
const compactGlyph: Record<WindowButton | "restore", LucideIcon> = {
  minimize: Minus,
  maximize: Square,
  restore: Copy,
  close: X,
};

/**
 * The main window's own buttons on one side of Sona's title bar, in the
 * desktop's order: Windows' caption buttons (46px wide, the bar's full
 * height, red Close) or Sona's compact ones elsewhere (28px, the top bar's
 * icon style). Close does what closing the window does (Settings → Desktop).
 */
export function WindowControls({ chrome, side }: { chrome: WindowChrome; side: "start" | "end" }) {
  const { t } = useLingui();
  const maximised = useWindowStateStore((s) => s.state.maximised);
  const active = useWindowActive();
  const list = side === "start" ? chrome.start : chrome.end;
  if (list.length === 0) return null;

  const button = (b: WindowButton) => {
    const glyph = b === "maximize" && maximised ? "restore" : b;
    const label = {
      minimize: t`Minimise`,
      maximize: maximised ? t`Restore` : t`Maximise`,
      close: t`Close`,
    }[b];
    const action = () => {
      if (b === "minimize")
        void run(t`Couldn't minimise the window`, DesktopService.MinimiseWindow());
      else if (b === "maximize")
        void run(t`Couldn't resize the window`, DesktopService.ToggleMaximiseWindow());
      else void run(t`Couldn't close the window`, DesktopService.CloseWindow());
    };
    // A double click on a button is two presses, not the title bar's maximise.
    const stop = (e: MouseEvent) => e.stopPropagation();
    if (chrome.captions) {
      return (
        <button
          key={b}
          type="button"
          aria-label={label}
          title={label}
          onClick={action}
          onDoubleClick={stop}
          className={cn(
            "grid h-full w-[46px] place-items-center text-ink transition-colors duration-100 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring",
            b === "close"
              ? "hover:bg-[#c42b1c] hover:text-white active:bg-[#c42b1c]/90 active:text-white"
              : "hover:bg-hover active:bg-active",
            !active && "text-ink-faint",
          )}
        >
          {captionGlyph[glyph]}
        </button>
      );
    }
    const Icon = compactGlyph[glyph];
    return (
      <button
        key={b}
        type="button"
        aria-label={label}
        title={label}
        onClick={action}
        onDoubleClick={stop}
        className={cn(
          "grid size-7 place-items-center rounded-md transition-colors duration-100 hover:bg-hover hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring active:bg-active",
          active ? "text-ink-muted" : "text-ink-faint",
        )}
      >
        <Icon
          aria-hidden
          className={glyph === "restore" ? "size-3.5" : "size-4"}
          strokeWidth={1.75}
        />
      </button>
    );
  };

  return (
    <div
      role="group"
      aria-label={t`Window`}
      className={cn("flex shrink-0 items-center", chrome.captions ? "h-full" : "gap-1")}
    >
      {list.map(button)}
    </div>
  );
}
