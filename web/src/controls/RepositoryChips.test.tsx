import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RepositoryChips } from "./RepositoryChips";

describe("RepositoryChips", () => {
  it("lists each Repository Name as a chip, in order", () => {
    render(<RepositoryChips names={["acme", "api"]} />);
    const list = screen.getByRole("list", { name: "Changed Repositories" });
    expect([...list.querySelectorAll("[role=listitem]")].map((c) => c.textContent)).toEqual(["acme", "api"]);
    expect(screen.getByText("api").className).toBe("label repo-chip");
  });

  it("shows nothing without Repositories", () => {
    const { container } = render(<RepositoryChips names={[]} />);
    expect(container).toBeEmptyDOMElement();
    render(<RepositoryChips />);
    expect(screen.queryByRole("list", { name: "Changed Repositories" })).toBeNull();
  });
});
