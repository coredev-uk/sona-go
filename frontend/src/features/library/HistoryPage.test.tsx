import type { ListeningHistory, Song } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { HistoryPage } from "./HistoryPage";

const history = vi.fn<(days: number) => Promise<ListeningHistory>>();
const playSongs = vi.fn();

vi.mock("@bindings/services", () => ({ LibraryService: { History: (d: number) => history(d) } }));
vi.mock("@/features/player", () => ({ playSongs: (...a: unknown[]) => playSongs(...a) }));
vi.mock("@/features/auth", () => ({
  useAuth: () => ({ data: { state: "signed_in" } }),
  isSignedOut: () => false,
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
  useNavigate: () => () => {},
}));

const song = (id: string, name: string) =>
  ({
    id,
    name,
    source: "catalog",
    playable: true,
    artistName: "A",
    albumName: "B",
    art: {},
  }) as Song;

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <HistoryPage />
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("HistoryPage", () => {
  it("lists each day's songs and plays a day from the chosen song", async () => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const yesterday = today.getTime() - 86_400_000;
    const a = song("1", "Alpha");
    const b = song("2", "Beta");
    history.mockResolvedValue({
      days: [
        {
          day: today.getTime(),
          entries: [
            { playedAt: today.getTime() + 5000, song: a, playedMs: 1, context: "" },
            { playedAt: 0, song: b, playedMs: 0, context: "" },
            { playedAt: today.getTime() + 1000, song: a, playedMs: 1, context: "" },
          ],
        },
        {
          day: yesterday,
          entries: [{ playedAt: yesterday + 1, song: b, playedMs: 1, context: "" }],
        },
      ],
      since: yesterday,
      more: false,
      apple: true,
    });
    renderPage();

    const todayList = await screen.findByRole("grid", { name: "Played Today" });
    expect(screen.getByRole("heading", { name: "Yesterday" })).toBeTruthy();
    expect(screen.getByText("2 songs, 3 plays · 1 from other devices")).toBeTruthy();
    expect(screen.getByText("On this computer and your other devices")).toBeTruthy();
    expect(history).toHaveBeenCalledWith(30);

    fireEvent.doubleClick(within(todayList).getByText("Beta"));
    expect(playSongs).toHaveBeenCalledWith([a, b], 1, "History");
  });

  it("says when nothing was played", async () => {
    history.mockResolvedValue({ days: [], since: 0, more: false, apple: true });
    renderPage();
    expect(await screen.findByText("Nothing played yet")).toBeTruthy();
  });
});
