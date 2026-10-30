import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { CircleAlert, WifiOff } from "lucide-react";

import { useAuth } from "@/features/auth";

/**
 * One line under the top bar when the account cannot reach Apple: offline,
 * a token Apple stopped accepting, or a rejected developer token. The page
 * below keeps rendering from SQLite.
 */
export function StatusBanner() {
  const { t } = useLingui();
  const { data } = useAuth();
  if (!data) return null;

  let icon = <CircleAlert aria-hidden className="size-4 shrink-0 text-danger" strokeWidth={1.75} />;
  let text: string;
  let action: { label: string; section: "account" | "diagnostics" } | undefined;

  if (data.state === "needs_reauth") {
    text = t`Apple Music asked Sona to sign in again. Your library is still here.`;
    action = { label: t`Sign in`, section: "account" };
  } else if (data.state === "error") {
    const reason = data.lastError?.replace(/^[a-z]+: /, "");
    text = reason
      ? t`Apple Music isn't accepting Sona's requests: ${reason}`
      : t`Apple Music isn't accepting Sona's requests.`;
    action = { label: t`Diagnostics`, section: "diagnostics" };
  } else if (data.offline) {
    icon = <WifiOff aria-hidden className="size-4 shrink-0 text-ink-muted" strokeWidth={1.75} />;
    text = t`Offline. Showing your library from this computer; Sona reconnects by itself.`;
  } else {
    return null;
  }

  return (
    <div
      role="status"
      className="flex h-9 shrink-0 items-center gap-2.5 border-t border-line bg-hover px-8 text-sub text-ink max-[960px]:px-6"
    >
      {icon}
      <span className="min-w-0 flex-1 truncate">{text}</span>
      {action && (
        <Link
          to="/settings"
          search={{ section: action.section }}
          className="shrink-0 font-medium underline underline-offset-2 hover:text-ink"
        >
          {action.label}
        </Link>
      )}
    </div>
  );
}
