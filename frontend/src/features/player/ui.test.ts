import type { QueueItem } from "@bindings/services";
import { afterEach, describe, expect, it } from "vitest";

import { emptyState, usePlayback } from "./playback";
import { queueShown, shownPane, toggleQueueView, useUI } from "./ui";

const song = { id: "1", catalogId: "1", live: false } as QueueItem;
const upload = { id: "i.abc", catalogId: "", live: false } as QueueItem;
const station = { id: "ra.1", catalogId: "", live: true } as QueueItem;

afterEach(() => {
  useUI.setState({ overlay: "none", pane: "lyrics", queueOpen: false, fromRoom: false });
  usePlayback.setState({ ...emptyState });
});

describe("shownPane", () => {
  it("shows the asked pane for a catalogue song", () => {
    expect(shownPane("lyrics", song)).toBe("lyrics");
    expect(shownPane("queue", song)).toBe("queue");
  });

  it("shows Up next for what has no lyrics: a station, a library upload, nothing", () => {
    expect(shownPane("lyrics", station)).toBe("queue");
    expect(shownPane("lyrics", upload)).toBe("queue");
    expect(shownPane("lyrics", null)).toBe("queue");
  });
});

describe("the view's panes", () => {
  it("reopens on the pane it showed last unless one is asked for", () => {
    const ui = useUI.getState();
    ui.openNowPlaying("queue");
    ui.closeOverlay();
    ui.openNowPlaying();
    expect(useUI.getState().pane).toBe("queue");
    useUI.getState().openNowPlaying("lyrics");
    expect(useUI.getState().pane).toBe("lyrics");
  });

  it("puts full screen back on the lyrics, and leaving it returns to the view", () => {
    useUI.getState().openNowPlaying("queue");
    useUI.getState().openRoom();
    expect(useUI.getState()).toMatchObject({ overlay: "room", pane: "lyrics" });
    useUI.getState().closeOverlay();
    expect(useUI.getState()).toMatchObject({ overlay: "now-playing", fromRoom: true });
  });
});

describe("the queue toggle", () => {
  it("shows and hides the queue beside the page while the view is closed", () => {
    usePlayback.setState({ current: song });
    toggleQueueView();
    expect(useUI.getState().queueOpen).toBe(true);
    expect(queueShown(useUI.getState(), song)).toBe(true);
    toggleQueueView();
    expect(useUI.getState().queueOpen).toBe(false);
  });

  it("turns the open view to Up next, then closes it, leaving the aside alone", () => {
    usePlayback.setState({ current: song });
    useUI.getState().openNowPlaying("lyrics");
    expect(queueShown(useUI.getState(), song)).toBe(false);

    toggleQueueView();
    expect(useUI.getState()).toMatchObject({ overlay: "now-playing", pane: "queue" });
    expect(queueShown(useUI.getState(), song)).toBe(true);

    toggleQueueView();
    expect(useUI.getState()).toMatchObject({ overlay: "none", queueOpen: false });
  });

  it("closes the view of a station, whose only pane is Up next", () => {
    usePlayback.setState({ current: station });
    useUI.getState().openNowPlaying("lyrics");
    expect(queueShown(useUI.getState(), station)).toBe(true);
    toggleQueueView();
    expect(useUI.getState().overlay).toBe("none");
  });

  it("does not count the aside as shown while the view covers it", () => {
    useUI.setState({ queueOpen: true });
    useUI.getState().openNowPlaying("lyrics");
    expect(queueShown(useUI.getState(), song)).toBe(false);
  });
});
