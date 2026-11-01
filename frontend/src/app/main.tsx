import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "@/shared/styles/index.css";

import { bindAppearance } from "@/features/settings";
import { initI18n } from "@/shared/i18n";
import { wrapTabFocus } from "@/shared/lib/focus-wrap";
import { shareEventDispatch } from "@/shared/lib/go-events";
import { mirrorModalInert } from "@/shared/lib/modal-inert";
import { trackPageFocus } from "@/shared/lib/page-focus";

import { registerModuleActions, registerModuleShortcuts } from "./compose";
import { AppProviders } from "./providers";
import { router } from "./router";

shareEventDispatch();
// Before the first render, so the first frame already has the right theme
// and language.
initI18n();
bindAppearance();
registerModuleActions();
registerModuleShortcuts();
mirrorModalInert();
trackPageFocus();
wrapTabFocus();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AppProviders>
      <RouterProvider router={router} />
    </AppProviders>
  </StrictMode>,
);
