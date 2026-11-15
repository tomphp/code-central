// The shared Markdown renderer (ticket 47): GFM, display-only task lists,
// links that open in a new tab, and raw HTML kept as text.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { IdLinksContext, NO_ID_LINKS } from "./idLinks";
import { Markdown } from "./Markdown";
import { readLocation } from "./url";

describe("Markdown", () => {
  it("renders a heading", () => {
    render(<Markdown>{"## Build"}</Markdown>);
    expect(screen.getByRole("heading", { level: 2, name: "Build" })).toBeInTheDocument();
  });

  it("renders GFM tables and strikethrough", () => {
    const { container } = render(<Markdown>{"| a | b |\n| - | - |\n| 1 | 2 |\n\n~~gone~~"}</Markdown>);
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "2" })).toBeInTheDocument();
    expect(container.querySelector("del")).toHaveTextContent("gone");
  });

  it("renders a list", () => {
    render(<Markdown>{"- one\n- two"}</Markdown>);
    expect(screen.getAllByRole("listitem").map((li) => li.textContent)).toEqual(["one", "two"]);
  });

  it("renders task-list items as disabled checkboxes, checked or not", () => {
    render(<Markdown>{"- [ ] todo\n- [x] done"}</Markdown>);
    const [todo, done] = screen.getAllByRole("checkbox");
    expect(todo).not.toBeChecked();
    expect(done).toBeChecked();
    expect(todo).toBeDisabled();
    expect(done).toBeDisabled();
  });

  it("opens links in a new tab without handing over the opener", () => {
    render(<Markdown>{"See [the spec](https://example.com/spec)."}</Markdown>);
    const a = screen.getByRole("link", { name: "the spec" });
    expect(a).toHaveAttribute("href", "https://example.com/spec");
    expect(a).toHaveAttribute("target", "_blank");
    expect(a).toHaveAttribute("rel", "noopener noreferrer");
  });

  describe("ID Links", () => {
    it("link a Spec or Ticket's ID to the Inbox address, in place of a new tab, with no provider", () => {
      render(<Markdown>{"See [code-central-706](code-central-706)."}</Markdown>);
      const a = screen.getByRole("link", { name: "code-central-706" });
      expect(a).toHaveAttribute("href", "?view=inbox&open=code-central-706");
      expect(a).not.toHaveAttribute("target");
      expect(a).not.toHaveAttribute("rel");
    });

    it("navigate in place on a plain click and leave a modified click to the browser", () => {
      const navigate = vi.fn();
      const loc = readLocation("?view=board&project=acme");
      render(
        <IdLinksContext.Provider value={{ loc, navigate }}>
          <Markdown>{"[acme-2](acme-2)"}</Markdown>
        </IdLinksContext.Provider>,
      );
      const a = screen.getByRole("link", { name: "acme-2" });
      expect(a).toHaveAttribute("href", "?view=board&project=acme&popup=acme-2");
      fireEvent.click(a, { ctrlKey: true });
      expect(navigate).not.toHaveBeenCalled();
      fireEvent.click(a);
      expect(navigate).toHaveBeenCalledWith({ popup: "acme-2" });
    });

    it("open in a new tab in the edit preview's context", () => {
      render(
        <IdLinksContext.Provider value={{ ...NO_ID_LINKS, newTab: true }}>
          <Markdown>{"[acme-2](acme-2)"}</Markdown>
        </IdLinksContext.Provider>,
      );
      const a = screen.getByRole("link", { name: "acme-2" });
      expect(a).toHaveAttribute("target", "_blank");
      expect(a).toHaveAttribute("rel", "noopener noreferrer");
    });

    it.each(["docs/acme-2", "#2", "https://example.com/acme-2", "acme-2#top", "acme-2?x=1", "acme-0", "acme", "./acme-2"])(
      "are not made of %s",
      (target) => {
        render(<Markdown>{`[x](${target})`}</Markdown>);
        const a = screen.getByRole("link", { name: "x" });
        expect(a).toHaveAttribute("target", "_blank");
      },
    );

    it("leave a bare ID and an ID in a code span as text", () => {
      const { container } = render(<Markdown>{"acme-2 and `acme-3`"}</Markdown>);
      expect(screen.queryByRole("link")).toBeNull();
      expect(container).toHaveTextContent("acme-2 and acme-3");
    });
  });

  it("shows raw HTML as text and never renders it", () => {
    const { container } = render(
      <Markdown>{'<script>alert(1)</script>\n\nHi <img src=x onerror="alert(2)"> there'}</Markdown>,
    );
    expect(container.querySelector("script, img")).toBeNull();
    expect(container).toHaveTextContent("<script>alert(1)</script>");
    expect(container).toHaveTextContent('Hi <img src=x onerror="alert(2)"> there');
  });
});
