import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { PersonAvatar } from "./PersonAvatar";

afterEach(cleanup);

const photo = "/art/0123456789abcdef0123456789abcdef/96x96.jpg";

describe("PersonAvatar", () => {
  it("shows the picture at the exact size, decorative", () => {
    const { container } = render(<PersonAvatar name="Thom Yorke" src={photo} size={40} />);
    const img = container.querySelector("img")!;
    expect(img.getAttribute("src")).toBe("/art/0123456789abcdef0123456789abcdef/40x40.jpg");
    expect(img.getAttribute("alt")).toBe("");
    expect(container.querySelector("[aria-hidden='true']")?.contains(img)).toBe(true);
    expect(container.textContent).toBe("");
  });

  it("draws the initials without a picture", () => {
    const { container } = render(<PersonAvatar name="Colin Greenwood" size={40} />);
    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toBe("CG");
    expect(container.querySelector("[aria-hidden='true']")?.textContent).toBe("CG");
  });

  it("falls back to the initials when the picture fails", () => {
    const { container } = render(<PersonAvatar name="Nigel Godrich" src={photo} size={40} />);
    fireEvent.error(container.querySelector("img")!);
    expect(container.textContent).toBe("NG");
  });
});
