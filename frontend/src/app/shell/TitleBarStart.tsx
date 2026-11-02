import { Trans } from "@lingui/react/macro";

import { SonaMark } from "@/shared/brand/SonaMark";
import { cn } from "@/shared/lib/utils";

import { titleBarRegion, useWindowActive, type WindowChrome } from "./window-chrome";
import { WindowControls } from "./WindowControls";

/**
 * The title bar's leading end: macOS's buttons (the room kept for them) or
 * Sona's leading ones, then the mark and the wordmark, which dim while the
 * window is inactive. The sidebar's header.
 */
export function TitleBarStart({ chrome, className }: { chrome: WindowChrome; className?: string }) {
  const active = useWindowActive();
  const region = titleBarRegion(chrome);
  const leading = chrome.start.length > 0;
  return (
    <div
      {...region}
      style={{
        ...region.style,
        height: chrome.height,
        paddingInlineStart: chrome.startRoom || (leading ? 8 : 22),
      }}
      className={cn("flex shrink-0 items-center gap-2 select-none", className)}
    >
      {leading && (
        <div className="me-3">
          <WindowControls chrome={chrome} side="start" />
        </div>
      )}
      <SonaMark />
      <span
        className={cn(
          "text-heading font-bold tracking-tight transition-colors duration-100",
          chrome.kind !== "none" && !active ? "text-ink-faint" : "text-ink",
        )}
      >
        <Trans>Sona</Trans>
      </span>
    </div>
  );
}
