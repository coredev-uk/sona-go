import { useLingui } from "@lingui/react/macro";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, type ReactNode } from "react";

import { useAuth } from "@/features/auth";
import { useAppearance, useSettings } from "@/features/settings";
import { followAccountLanguage, followLanguageSetting, I18nRoot } from "@/shared/i18n";
import { capabilitiesQuery } from "@/shared/lib/capabilities";
import { Toaster } from "@/shared/ui/sonner";
import { TooltipProvider } from "@/shared/ui/tooltip";

/**
 * Query defaults for Go-backed data: bindings resolve from SQLite first, and
 * Go events invalidate queries, so background refetching is off.
 */
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: Infinity,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});
// Asked before the first render, so controls gated on a capability (the
// player bar's volume) are usually there in the first frame.
void queryClient.prefetchQuery(capabilitiesQuery);

/** Providers above the router: the active locale and the query client. */
export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <I18nRoot>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </I18nRoot>
  );
}

/**
 * Providers inside the shell: tooltips, and the toaster above the
 * now-playing bar. The UI language follows the Language setting, else the
 * account's storefront and the system.
 */
export function ShellProviders({ children }: { children: ReactNode }) {
  const { t } = useLingui();
  const theme = useAppearance((s) => s.theme);
  const language = useAuth().data?.language ?? "";
  const chosen = useSettings().data?.language ?? "";
  useEffect(() => followAccountLanguage(language), [language]);
  useEffect(() => followLanguageSetting(chosen), [chosen]);
  return (
    <TooltipProvider delayDuration={400}>
      {children}
      <Toaster
        theme={theme}
        position="bottom-right"
        offset={{ bottom: 88, right: 16 }}
        containerAriaLabel={t`Notifications`}
      />
    </TooltipProvider>
  );
}
