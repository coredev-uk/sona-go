import { Events } from "@wailsio/runtime";
import { useRouter } from "@tanstack/react-router";
import { useEffect } from "react";

/**
 * The mouse's back and forward buttons go through the page's history, as the
 * top bar's arrows and Alt+←/→ do. Go sends `window:navigate` for them
 * (Linux: WebKitGTK gives the page neither button), so there is no
 * `mouseup` listener here.
 */
export function useMouseNavigation() {
  const { history } = useRouter();
  useEffect(
    () =>
      Events.On("window:navigate", ({ data }) => {
        if (data === "forward") history.forward();
        else history.back();
      }),
    [history],
  );
}
