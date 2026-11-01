import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { PreviewMark } from "./Badges";

afterEach(cleanup);

const tooltip = "Apple Music's 30-second preview, not the full song";

describe("PreviewMark", () => {
  it("reads as Preview, described by its tooltip without relying on title", () => {
    render(<PreviewMark />, { wrapper: I18nRoot });
    const text = screen.getByText("Preview");
    const box = text.parentElement!;
    expect(box.getAttribute("title")).toBe(tooltip);
    // WebKitGTK exposes no description from this box's title (jsdom's
    // accessible-name code would fall back to it), so the description must
    // hold without it.
    box.removeAttribute("title");
    expect(screen.getByRole("generic", { description: tooltip })).toBe(text);
    // The describing element is hidden, so the text is not read twice.
    expect(screen.getByText(tooltip).hidden).toBe(true);
  });
});
