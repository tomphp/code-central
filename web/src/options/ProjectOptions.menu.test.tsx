// Project Options: the section menu.
import { within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderApp } from "../worldTesting";
import { acme, page } from "./ProjectOptions.testutil";

describe("Project Options", () => {
  it("has a section menu whose entries jump to their sections", async () => {
    const { w } = await acme();
    await renderApp(w, "/?view=project-options&project=acme");

    const menu = within(page()).getByRole("navigation", { name: "Sections" });
    const links = within(menu).getAllByRole("link");
    expect(links.map((l) => l.textContent)).toEqual([
      "Repositories",
      "Landing Method",
      "CI",
      "GitHub Issues",
      "Azure Boards",
      "Models",
      "Stage Guides",
      "Auto-accept",
      "Focus",
    ]);

    const scrolled: string[] = [];
    Element.prototype.scrollIntoView = vi.fn(function (this: Element) {
      scrolled.push(this.id);
    });
    for (const link of links) {
      const id = link.getAttribute("href")!.slice(1);
      const section = page().querySelector(`#${id}`);
      expect(section).not.toBeNull();
      expect(section!.querySelector("legend")!.textContent).toBe(
        link.textContent,
      );
      await userEvent.click(link);
      expect(scrolled.pop()).toBe(id);
    }
    expect(window.location.search).toBe("?view=project-options&project=acme");
  });

});
