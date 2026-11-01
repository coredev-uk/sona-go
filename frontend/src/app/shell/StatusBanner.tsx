import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { CircleAlert, LoaderCircle, WifiOff } from "lucide-react";
import type { ReactNode } from "react";

import { KeyringRetry, useAuth } from "@/features/auth";

const ACTION_CLS =
  "shrink-0 font-medium underline underline-offset-2 hover:text-ink aria-disabled:text-ink-muted aria-disabled:no-underline";

/**
 * One line under the top bar when the account can't reach Apple (offline,
 * a token Apple stopped accepting, a rejected developer token) or waits for
 * the keyring (the sign-in being read or saved, or a keyring that didn't
 * answer). The page below keeps rendering from SQLite.
 */
export function StatusBanner() {
  const { t } = useLingui();
  const { data } = useAuth();
  if (!data) return null;

  let icon = <CircleAlert aria-hidden className="size-4 shrink-0 text-danger" strokeWidth={1.75} />;
  let text: string;
  let action: ReactNode = null;

  if (data.keyring === "waiting") {
    icon = (
      <LoaderCircle
        aria-hidden
        className="size-4 shrink-0 animate-spin text-ink-muted motion-reduce:animate-none"
        strokeWidth={1.75}
      />
    );
    if (data.state === "restoring") {
      text = t`Signing in… Sona is waiting for your keyring; unlock it if it asks.`;
      action = <KeyringRetry status={data} className={ACTION_CLS} />;
    } else {
      text = t`Saving your sign-in in your keyring; unlock it if it asks.`;
    }
  } else if (data.keyring === "unavailable") {
    text = t`Sona couldn't read your sign-in from your keyring, so you're signed out.`;
    action = <KeyringRetry status={data} className={ACTION_CLS} />;
  } else if (data.state === "needs_reauth") {
    text = t`Apple Music asked Sona to sign in again. Your library is still here.`;
    action = <SettingsLink label={t`Sign in`} section="account" />;
  } else if (data.state === "error") {
    const reason = data.lastError?.replace(/^[a-z]+: /, "");
    text = reason
      ? t`Apple Music isn't accepting Sona's requests: ${reason}`
      : t`Apple Music isn't accepting Sona's requests.`;
    action = <SettingsLink label={t`Diagnostics`} section="diagnostics" />;
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
      {action}
    </div>
  );
}

function SettingsLink({ label, section }: { label: string; section: "account" | "diagnostics" }) {
  return (
    <Link to="/settings" search={{ section }} className={ACTION_CLS}>
      {label}
    </Link>
  );
}
