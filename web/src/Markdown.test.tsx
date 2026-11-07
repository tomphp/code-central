// The shared Markdown renderer (ticket 47): GFM, display-only task lists,
// links that open in a new tab, and raw HTML kept as text.
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Markdown } from "./Markdown";

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

  it("shows raw HTML as text and never renders it", () => {
    const { container } = render(
      <Markdown>{'<script>alert(1)</script>\n\nHi <img src=x onerror="alert(2)"> there'}</Markdown>,
    );
    expect(container.querySelector("script, img")).toBeNull();
    expect(container).toHaveTextContent("<script>alert(1)</script>");
    expect(container).toHaveTextContent('Hi <img src=x onerror="alert(2)"> there');
  });
});
