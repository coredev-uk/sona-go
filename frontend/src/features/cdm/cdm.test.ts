import { readFileSync } from "node:fs";
import path from "node:path";

import type { CDMStatus } from "@bindings/services";
import { QueryClient } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { keys } from "@/shared/lib/keys";
import type { EventContext } from "@/shared/module";

import { cdmModule } from "./index";
import { cdmProblem, hasCdmProblem } from "./messages";
import { applyCdmStatus, canRetryCdm, cdmPercent, cdmProgress, retryCdm, useCdm } from "./state";

const retry = vi.hoisted(() => vi.fn<() => Promise<void>>());
const snapshot = vi.hoisted(() => vi.fn<() => Promise<unknown>>());
vi.mock("@bindings/services", () => ({
  CDMService: { Retry: () => retry(), Status: () => snapshot() },
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

function status(patch: Partial<CDMStatus> = {}): CDMStatus {
  return {
    state: "ready",
    source: "download",
    version: "4.10.3112.0",
    path: "/data/widevine/4.10.3112.0/linux_x64/libwidevinecdm.so",
    code: "",
    received: 0,
    total: 0,
    updating: false,
    checkedAt: 0,
    ...patch,
  };
}

function context() {
  const invalidate = vi.fn<EventContext["invalidate"]>();
  const ctx = { qc: new QueryClient(), invalidate, navigate: vi.fn() } as unknown as EventContext;
  return { ctx, invalidate };
}

beforeEach(() => {
  useCdm.setState({ status: null });
  retry.mockReset();
});

describe("cdm:status", () => {
  const handle = cdmModule.eventHandlers!["cdm:status"]!;

  it("mirrors Go's status", () => {
    const { ctx } = context();
    handle(status({ state: "downloading", received: 10, total: 100 }), ctx);
    expect(useCdm.getState().status).toMatchObject({ state: "downloading", received: 10 });
  });

  it("refreshes Diagnostics when the state changes, not on every progress step", () => {
    const { ctx, invalidate } = context();
    handle(status({ state: "downloading", received: 0, total: 100 }), ctx);
    expect(invalidate).toHaveBeenCalledTimes(1);
    expect(invalidate).toHaveBeenLastCalledWith([keys.diagnostics]);
    handle(status({ state: "downloading", received: 40, total: 100 }), ctx);
    handle(status({ state: "downloading", received: 80, total: 100 }), ctx);
    expect(invalidate).toHaveBeenCalledTimes(1);
    handle(status({ state: "ready" }), ctx);
    expect(invalidate).toHaveBeenCalledTimes(2);
    handle(status({ state: "ready", code: "network" }), ctx);
    expect(invalidate).toHaveBeenCalledTimes(3);
  });

  it("reports whether more than the progress changed", () => {
    expect(applyCdmStatus(status({ state: "idle" }))).toBe(true);
    expect(applyCdmStatus(status({ state: "idle" }))).toBe(false);
    expect(applyCdmStatus(status({ state: "idle", version: "4.11" }))).toBe(true);
    expect(applyCdmStatus(status({ state: "idle", version: "4.11", source: "override" }))).toBe(
      true,
    );
  });
});

describe("init", () => {
  it("fills the mirror from CDMService.Status", async () => {
    const { ctx } = context();
    snapshot.mockResolvedValueOnce(status({ state: "idle", version: "" }));
    cdmModule.init!(ctx);
    await vi.waitFor(() => expect(useCdm.getState().status?.state).toBe("idle"));
  });

  it("keeps a cdm:status that lands before Status answers", async () => {
    const { ctx } = context();
    let resolve: (v: unknown) => void = () => {};
    const promise = new Promise<unknown>((r) => (resolve = r));
    snapshot.mockReturnValueOnce(promise);
    cdmModule.init!(ctx);
    cdmModule.eventHandlers!["cdm:status"]!(status({ state: "failed", code: "network" }), ctx);
    resolve(status({ state: "idle", version: "" }));
    await promise;
    await Promise.resolve();
    expect(useCdm.getState().status).toMatchObject({ state: "failed", code: "network" });
  });
});

describe("progress", () => {
  it("is a fraction and a percentage once the size is known", () => {
    expect(cdmProgress({ received: 0, total: 0 })).toBeNull();
    expect(cdmPercent({ received: 5, total: 0 })).toBe("");
    expect(cdmProgress({ received: 42, total: 100 })).toBe(0.42);
    expect(cdmPercent({ received: 42, total: 100 })).toBe("42%");
    expect(cdmProgress({ received: 150, total: 100 })).toBe(1);
  });
});

describe("canRetryCdm", () => {
  it("offers Retry after a failed download or update check", () => {
    expect(canRetryCdm(status({ state: "failed", code: "network" }))).toBe(true);
    expect(canRetryCdm(status({ state: "ready", code: "hash_mismatch" }))).toBe(true);
  });

  it("does not offer it where trying again cannot help", () => {
    expect(canRetryCdm(null)).toBe(false);
    expect(canRetryCdm(status())).toBe(false);
    expect(canRetryCdm(status({ state: "downloading" }))).toBe(false);
    expect(canRetryCdm(status({ state: "failed", code: "unsupported_platform" }))).toBe(false);
    expect(
      canRetryCdm(status({ state: "failed", source: "override", code: "override_missing" })),
    ).toBe(false);
    expect(canRetryCdm(status({ state: "ready", code: "network", updating: true }))).toBe(false);
  });

  it("asks Go to try again, and says so when it can't", async () => {
    retry.mockResolvedValueOnce(undefined);
    expect(await retryCdm()).toBe(true);
    retry.mockRejectedValueOnce(new Error("boom"));
    expect(await retryCdm()).toBe(false);
    expect(retry).toHaveBeenCalledTimes(2);
  });
});

describe("cdmProblem", () => {
  it("has wording for every failure code in internal/playback/cdm/errors.go", () => {
    const src = readFileSync(
      path.resolve(import.meta.dirname, "../../../../internal/playback/cdm/errors.go"),
      "utf8",
    );
    const codes = [...src.matchAll(/^\s*Code\w+\s*=\s*"([a-z0-9_]+)"/gm)].map((m) => m[1]);
    expect(codes.length).toBe(11);
    expect(codes.filter((c) => !hasCdmProblem(c))).toEqual([]);
  });

  it("words an unknown code generally instead of showing it", () => {
    expect(cdmProblem("network")).toContain("Google's download server");
    expect(cdmProblem("something_new")).toBe("Widevine couldn't be downloaded. Try again later.");
  });
});
