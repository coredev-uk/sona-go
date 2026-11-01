import type { Settings } from "@bindings/config";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import type { Shortcut } from "@/shared/module";
import { registerShortcuts } from "@/shared/shortcuts/registry";

import { ShortcutsSection } from "./Shortcuts";

vi.mock("@bindings/services", () => ({
  SystemService: {
    Settings: () => Promise.resolve({ shortcuts: {} } as unknown as Settings),
    UpdateSettings: vi.fn(),
  },
}));

const sc = (id: string, keys: string[]): Shortcut => ({
  id,
  label: () => id,
  group: "view",
  keys,
  run: () => {},
});

afterEach(cleanup);

describe("ShortcutsSection", () => {
  it("names each recorder by its action and the keycaps it shows (Label in Name)", async () => {
    registerShortcuts([
      sc("test.sidebar", ["Ctrl+B"]),
      sc("test.close", ["Escape"]),
      sc("test.unset", []),
    ]);
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <ShortcutsSection />
      </QueryClientProvider>,
      { wrapper: I18nRoot },
    );
    expect(await screen.findByRole("button", { name: "test.sidebar Ctrl B" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "test.close Esc" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "test.unset not set" })).toBeTruthy();
  });
});
