import { AuthService, type AuthStatus } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { ShieldAlert, User } from "lucide-react";
import { useId, useState, type ReactNode } from "react";

import { isForced, setPrivate, usePrivate } from "@/features/private";
import { openAccountDialog } from "@/shared/lib/account";
import { run } from "@/shared/lib/run";
import { cn } from "@/shared/lib/utils";
import { Avatar, AvatarFallback } from "@/shared/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { AccountDialog } from "./AccountDialog";
import { SignOutDialog } from "./Account";
import { hasAccount, needsAttention, useAuth } from "./queries";

/** One line for where the account stands, under "Apple Music" and in the avatar's description. */
function useStateLine(s: AuthStatus | undefined): string {
  const { t } = useLingui();
  if (!s) return "";
  if (hasAccount(s.state)) {
    if (s.keyring === "waiting")
      return t`Saving your sign-in in your keyring; unlock it if it asks.`;
    if (s.state === "validating") return t`Checking your sign-in…`;
    const store = s.storefrontName || s.storefrontId.toUpperCase();
    return store
      ? t({
          message: `Signed in · ${store}`,
          comment: "The account menu's status line: signed in, then the storefront's country",
        })
      : t`Signed in to Apple Music`;
  }
  switch (s.state) {
    case "restoring":
    case "signing_in":
      return t`Signing in…`;
    case "needs_reauth":
      return t`Apple stopped accepting Sona's sign-in`;
    case "error":
      return t`Apple Music isn't accepting Sona's requests`;
  }
  if (s.keyring === "unavailable") return t`Sona couldn't read your sign-in from your keyring`;
  return t({ message: "Not signed in", comment: "The account menu's status line" });
}

function Note({ icon, children }: { icon?: ReactNode; children: ReactNode }) {
  return (
    <p className="flex items-start gap-1.5 text-meta text-ink-muted">
      {icon}
      <span>{children}</span>
    </p>
  );
}

/**
 * The top bar's account: the avatar opens a menu with where the Apple Music
 * account stands (storefront, restricted content, offline, the keyring),
 * signing in or the account's settings (the account dialog), private
 * listening, and signing out. Enter, Space or a click open it; arrows move;
 * Escape closes it and returns focus to the avatar.
 */
export function AccountMenu() {
  const { t } = useLingui();
  const status = useAuth().data;
  const priv = usePrivate((s) => s.state);
  const [confirmSignOut, setConfirmSignOut] = useState(false);
  const stateId = useId();
  const privateId = useId();

  const line = useStateLine(status);
  const signedIn = hasAccount(status?.state);
  const attention = needsAttention(status);
  const keyringRetry =
    status !== undefined &&
    !signedIn &&
    status.state !== "restoring" &&
    (status.keyring === "unavailable" || status.unencrypted);
  const forced = isForced(priv);
  const privateNote = forced
    ? t`On because Sona was started with SONA_PRIVATE=1`
    : priv?.source === "development" && priv.on
      ? t`Private listening is on for this development build. Turning it off lasts until Sona quits.`
      : t({
          message: "Plays stay out of your listening history, scrobbles and Discord.",
          comment: "Under the account menu's Private listening switch",
        });

  return (
    <>
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                aria-label={t`Account`}
                aria-describedby={line ? stateId : undefined}
                className="relative rounded-full"
              >
                <Avatar className="size-7">
                  <AvatarFallback
                    className={cn(
                      "transition-colors duration-100",
                      signedIn ? "bg-active text-ink" : "bg-hover text-ink-muted",
                    )}
                  >
                    <User aria-hidden className="size-4" strokeWidth={1.75} />
                  </AvatarFallback>
                </Avatar>
                {attention && (
                  <span
                    aria-hidden
                    className="absolute -end-0.5 -top-0.5 size-2.5 rounded-full border-2 border-surface bg-danger"
                  />
                )}
                {line && (
                  <span id={stateId} hidden>
                    {line}
                  </span>
                )}
              </button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent className="max-w-72">
            <Trans>Account</Trans>
            {line && <span className="block text-ink-muted">{line}</span>}
          </TooltipContent>
        </Tooltip>
        <DropdownMenuContent
          align="end"
          className="w-72"
          aria-describedby={line ? stateId : undefined}
        >
          <div className="flex flex-col gap-1 px-2 pt-1.5 pb-2">
            <p className="text-ui font-medium text-ink">
              <Trans>Apple Music</Trans>
            </p>
            {line && <Note>{line}</Note>}
            {signedIn && status?.offline && (
              <Note>
                {t({
                  message: "Offline. Sona keeps using your saved sign-in.",
                  comment: "In the account menu while Apple can't be reached",
                })}
              </Note>
            )}
            {signedIn && status?.restricted && (
              <Note>
                <Trans>Explicit content is restricted on this account; Sona hides it.</Trans>
              </Note>
            )}
            {status?.unencrypted && (
              <Note
                icon={
                  <ShieldAlert
                    aria-hidden
                    className="mt-px size-3.5 shrink-0 text-danger"
                    strokeWidth={1.75}
                  />
                }
              >
                {signedIn ? (
                  <Trans>Your token is stored unencrypted</Trans>
                ) : (
                  <Trans>Signing in now stores your token unencrypted</Trans>
                )}
              </Note>
            )}
          </div>
          <DropdownMenuSeparator />
          {signedIn ? (
            <DropdownMenuItem onSelect={openAccountDialog}>
              <Trans>Account settings</Trans>
            </DropdownMenuItem>
          ) : (
            status &&
            status.state !== "restoring" && (
              <DropdownMenuItem onSelect={openAccountDialog}>
                <Trans>Sign in</Trans>
              </DropdownMenuItem>
            )
          )}
          {signedIn && status?.offline && (
            <DropdownMenuItem
              onSelect={() => void run(t`Couldn't reach Apple`, AuthService.Revalidate())}
            >
              <Trans>Check now</Trans>
            </DropdownMenuItem>
          )}
          {keyringRetry && (
            <DropdownMenuItem onSelect={() => void AuthService.RetryKeyring()}>
              {t({
                message: "Try again",
                comment:
                  "Reads the sign-in from the system keyring again. Apple's wording: ASE.Web.Music.Error.TryAgain",
              })}
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuCheckboxItem
            checked={priv?.on ?? false}
            disabled={!priv || forced}
            aria-describedby={privateId}
            // Stays open: the tick is the confirmation.
            onSelect={(e) => e.preventDefault()}
            onCheckedChange={(on) => void setPrivate(on)}
          >
            {t({
              message: "Private listening",
              comment:
                "Switch name. No Apple Music web string; Apple's apps offer the inverse, 'Use Listening History'.",
            })}
          </DropdownMenuCheckboxItem>
          <p id={privateId} className="ps-8 pe-2 pb-1.5 text-meta text-ink-muted">
            {privateNote}
          </p>
          {signedIn && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onSelect={() => setConfirmSignOut(true)}>
                <Trans>Sign out</Trans>
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      <SignOutDialog open={confirmSignOut} onOpenChange={setConfirmSignOut} />
      <AccountDialog />
    </>
  );
}
