import { SystemService } from "@bindings/services";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Events } from "@wailsio/runtime";
import { useEffect } from "react";

import type { EventContext, GoEventHandler, GoEventName } from "@/shared/module";

import { modules } from "./modules";
import { routeForOpen } from "./open";

let painted = false;

/** Opens a music:// / music.apple.com link (app:open) on its page. */
const openLink: GoEventHandler<"app:open"> = (data, { navigate }) => {
  const route = routeForOpen(data);
  if (route) void navigate(route);
};

/**
 * Subscribes every module's Go event handlers (and the app's own app:open)
 * and returns the unsubscribe function. Payloads replace the matching cache
 * or store; each module's pure invalidation plans decide what else is stale.
 */
function bindGoEvents(ctx: EventContext): () => void {
  const handlers = [
    ...modules.flatMap((m) => Object.entries(m.eventHandlers ?? {})),
    ["app:open", openLink],
  ] as [GoEventName, GoEventHandler<GoEventName>][];
  const offs = handlers.map(([name, handler]) => Events.On(name, ({ data }) => handler(data, ctx)));
  return () => offs.forEach((off) => off());
}

/**
 * Connects the shell to Go once: event listeners first, then each module's
 * initial snapshots (`init`), then FirstPaint after the browser has drawn
 * the shell, which releases Go's deferred network work (token validation,
 * stale-library sync, catalogue prune) and any queued app:open links.
 */
export function useGoBridge() {
  const qc = useQueryClient();
  const navigate = useNavigate();

  useEffect(() => {
    const ctx: EventContext = {
      qc,
      navigate,
      invalidate: (list) => {
        for (const queryKey of list) void qc.invalidateQueries({ queryKey });
      },
    };
    const unbind = bindGoEvents(ctx);
    for (const m of modules) m.init?.(ctx);

    const fire = () => {
      if (painted) return;
      painted = true;
      void SystemService.FirstPaint();
    };
    // Two frames: the first runs before paint, the second after it. WebKit
    // throttles frames for a window that starts hidden, so a short timer
    // releases Go's network work there too.
    let raf = requestAnimationFrame(() => (raf = requestAnimationFrame(fire)));
    const timer = window.setTimeout(fire, 300);
    return () => {
      cancelAnimationFrame(raf);
      window.clearTimeout(timer);
      unbind();
    };
  }, [qc, navigate]);
}
