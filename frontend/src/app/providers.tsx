import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { useAppearance } from "@/features/settings";
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

/** Providers above the router. */
export function AppProviders({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

/** Providers inside the shell: tooltips, and the toaster above the now-playing bar. */
export function ShellProviders({ children }: { children: ReactNode }) {
  const theme = useAppearance((s) => s.theme);
  return (
    <TooltipProvider delayDuration={400}>
      {children}
      <Toaster theme={theme} position="bottom-right" offset={{ bottom: 88, right: 16 }} />
    </TooltipProvider>
  );
}
