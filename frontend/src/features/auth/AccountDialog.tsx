import { Trans } from "@lingui/react/macro";

import { closeAccountDialog, useAccountDialog } from "@/shared/lib/account";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/shared/ui/dialog";

import { AccountSection } from "./Account";

/**
 * The account dialog: signing in (the native form, the browser and token
 * fallbacks, the keyring's notices) or, signed in, the account's settings
 * and Sign out. Opened from the account menu, the palette, every "Sign in"
 * action and old links to Settings → Account (shared/lib/account.ts).
 * It stays open after a sign-in, which then shows the account it reached.
 */
export function AccountDialog() {
  const open = useAccountDialog((s) => s.open);
  return (
    <Dialog open={open} onOpenChange={(o) => !o && closeAccountDialog()}>
      <DialogContent
        aria-describedby={undefined}
        className="max-h-[calc(100%-2rem)] gap-5 overflow-y-auto sm:max-w-xl"
        // Signing in starts in the Apple Account field, past the keyring's notice.
        onOpenAutoFocus={(e) => {
          const field = document.getElementById("apple-account");
          if (!field) return;
          e.preventDefault();
          field.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle className="text-heading">
            <Trans>Account</Trans>
          </DialogTitle>
        </DialogHeader>
        <AccountSection />
      </DialogContent>
    </Dialog>
  );
}
