import type { SyncDone, SyncProgress, SyncStatus } from "@bindings/services";
import { QueryClient } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { syncStatusQuery } from "./queries";
import { useSync } from "./sync";

const syncStatus = vi.fn<() => Promise<SyncStatus>>();
vi.mock("@bindings/services", () => ({ LibraryService: { SyncStatus: () => syncStatus() } }));

const progress = (domain: string, done: number, total: number, finished = false) =>
  ({ domain, done, total, finished, errorCode: "" }) as unknown as SyncProgress;
const finished = { ok: true, errorCode: "", finishedAt: 1 } as SyncDone;
const status = (running: boolean) => ({ running, domains: [], lastSuccess: 0 }) as SyncStatus;

const sync = () => useSync.getState();

beforeEach(() => {
  useSync.setState({ running: false, current: null, last: null, events: 0 });
  syncStatus.mockReset();
});

describe("useSync", () => {
  it("stops after a first full sync, whatever domain it ended on", () => {
    // The events of a fresh profile's first sync, as Go sends them.
    for (const p of [
      progress("artists", 200, 414),
      progress("artists", 414, 414, true),
      progress("albums", 653, 653, true),
      progress("songs", 1075, 1075),
      progress("songs", 1075, 1075, true),
      progress("playlists", 25, 25, true),
      progress("playlist-tracks", 25, 25, true),
      progress("ratings", 1753, 1753, true),
    ]) {
      sync().progress(p);
    }
    expect(sync().running).toBe(true);
    sync().done(finished);
    expect(sync()).toMatchObject({ running: false, current: null, last: finished });
  });

  it("drops a status answer that sync:done overtook", () => {
    // Go answered "running" just before the sync ended, but the done event
    // reached the page before the answer did.
    sync().progress(progress("songs", 400, 1075));
    const at = sync().events;
    sync().done(finished);
    sync().snapshot(true, at);
    expect(sync().running).toBe(false);
  });

  it("drops a status answer that progress overtook", () => {
    const at = sync().events;
    sync().progress(progress("albums", 100, 653));
    sync().snapshot(false, at);
    expect(sync()).toMatchObject({ running: true, current: { domain: "albums", done: 100 } });
  });

  it("follows a status answer no event overtook", () => {
    // Started while a sync was already running: no event yet.
    sync().snapshot(true, sync().events);
    expect(sync()).toMatchObject({ running: true, current: null });
    // A sync whose sync:done never reached the page ends at the next answer.
    sync().progress(progress("songs", 1075, 1075, true));
    sync().snapshot(false, sync().events);
    expect(sync()).toMatchObject({ running: false, current: null });
  });
});

describe("syncStatusQuery", () => {
  it("does not restart the indicator when sync:done overtakes the start-up answer", async () => {
    const qc = new QueryClient();
    sync().progress(progress("ratings", 1728, 1753));
    // tsconfig's lib predates Promise.withResolvers.
    let answer!: (s: SyncStatus) => void;
    syncStatus.mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const fetched = qc.fetchQuery(syncStatusQuery);
    sync().done(finished);
    answer(status(true));
    await fetched;
    expect(sync().running).toBe(false);
  });

  it("brings the indicator up to date when nothing overtook the answer", async () => {
    const qc = new QueryClient();
    syncStatus.mockResolvedValueOnce(status(true));
    await qc.fetchQuery(syncStatusQuery);
    expect(sync().running).toBe(true);
    syncStatus.mockResolvedValueOnce(status(false));
    await qc.fetchQuery({ ...syncStatusQuery, staleTime: 0 });
    expect(sync().running).toBe(false);
  });
});
