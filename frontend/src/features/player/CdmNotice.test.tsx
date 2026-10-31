import type { CDMStatus } from "@bindings/services";
import { act, cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useCdm } from "@/features/cdm";
import { I18nRoot } from "@/shared/i18n";

import { CdmNotice } from "./CdmNotice";

vi.mock("@bindings/services", () => ({
  CDMService: { Retry: vi.fn(), Status: () => new Promise(() => {}) },
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, search }: { children: ReactNode; search: { section: string } }) => (
    <a href={`/settings?section=${search.section}`}>{children}</a>
  ),
}));

function status(patch: Partial<CDMStatus>): CDMStatus {
  return {
    state: "downloading",
    version: "",
    path: "",
    code: "",
    received: 0,
    total: 0,
    updating: false,
    checkedAt: 0,
    ...patch,
  };
}

function show(s: CDMStatus | null) {
  useCdm.setState({ status: s });
  render(<CdmNotice />, { wrapper: I18nRoot });
}

const link = () => screen.getByRole("link");

beforeEach(() => useCdm.setState({ status: null }));
afterEach(cleanup);

describe("CdmNotice", () => {
  it("says the song waits for the download, with its progress, and links to Settings → Playback", () => {
    show(status({ received: 21, total: 50 }));
    expect(link().textContent).toBe("Waiting for Widevine to download · 42%");
    expect(link().getAttribute("href")).toBe("/settings?section=playback");
    act(() => useCdm.setState({ status: status({ received: 50, total: 50 }) }));
    expect(link().textContent).toBe("Waiting for Widevine to download · 100%");
  });

  it("says it waits before Go has reported the CDM", () => {
    show(null);
    expect(link().textContent).toBe("Waiting for Widevine to download");
  });

  it("points a failed download to Settings for Retry, with the reason on hover", () => {
    show(status({ state: "failed", code: "network" }));
    expect(link().textContent).toBe("Couldn't get Widevine · Retry in Settings");
    expect(link().parentElement?.getAttribute("title")).toBe(
      "Sona couldn't reach Google's download server. Check your internet connection.",
    );
  });

  it("says songs can't play where Google makes no CDM", () => {
    show(status({ state: "failed", code: "unsupported_platform" }));
    expect(link().textContent).toBe("Songs can't play on this computer");
  });

  it("asks for another press of play once the CDM is ready", () => {
    show(status({ state: "ready", version: "4.10.3112.0" }));
    expect(screen.getByText("Widevine is ready. Press play to start.")).toBeTruthy();
    expect(screen.queryByRole("link")).toBeNull();
  });
});
