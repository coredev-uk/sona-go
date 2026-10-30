import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "@/shared/styles/index.css";

import { bindAppearance } from "@/features/settings";

import { registerModuleActions } from "./compose";
import { AppProviders } from "./providers";
import { router } from "./router";

// Before the first render, so the first frame already has the right theme.
bindAppearance();
registerModuleActions();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AppProviders>
      <RouterProvider router={router} />
    </AppProviders>
  </StrictMode>,
);
