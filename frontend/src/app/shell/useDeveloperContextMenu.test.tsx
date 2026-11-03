import type { Capabilities } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { linuxCapabilities } from "@/shared/testing/capabilities";

import { useDeveloperContextMenu } from "./useDeveloperContextMenu";

const capabilities = vi.fn<() => Promise<Capabilities>>();
vi.mock("@bindings/services", () => ({
  PlatformService: { Capabilities: () => capabilities() },
}));

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

const menu = () => document.documentElement.style.getPropertyValue("--default-contextmenu");

afterEach(() => {
  cleanup();
  document.documentElement.style.removeProperty("--default-contextmenu");
});

describe("useDeveloperContextMenu", () => {
  it("shows the web view's own menu (Inspect Element) while the inspector is on", async () => {
    capabilities.mockResolvedValue({ ...linuxCapabilities(), developerTools: true });
    const { unmount } = renderHook(() => useDeveloperContextMenu(), { wrapper });
    await waitFor(() => expect(menu()).toBe("show"));
    unmount();
    expect(menu()).toBe("");
  });

  it("leaves the menu to Wails' defaults while it is off", async () => {
    capabilities.mockResolvedValue(linuxCapabilities());
    renderHook(() => useDeveloperContextMenu(), { wrapper });
    await waitFor(() => expect(capabilities).toHaveBeenCalled());
    expect(menu()).toBe("");
  });
});
