import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { Tabs } from "./Tabs";

const tabs = [
  { value: "a", label: "Alpha" },
  { value: "b", label: "Beta" },
  { value: "c", label: "Gamma" },
] as const;

function Harness({ start = "a", onChange }: { start?: "a" | "b" | "c"; onChange?: (v: string) => void }) {
  const [value, setValue] = useState<"a" | "b" | "c">(start);
  return (
    <Tabs
      label="Parts"
      tabs={tabs}
      value={value}
      onChange={(v) => {
        onChange?.(v);
        setValue(v);
      }}
    >
      <p>Panel {value}</p>
    </Tabs>
  );
}

it("has tablist, tab and tabpanel roles, the selected tab labelling its panel", () => {
  render(<Harness start="b" />);
  expect(screen.getByRole("tablist", { name: "Parts" })).toBeInTheDocument();
  expect(screen.getAllByRole("tab").map((t) => t.textContent)).toEqual(["Alpha", "Beta", "Gamma"]);
  expect(screen.getByRole("tab", { name: "Beta" })).toHaveAttribute("aria-selected", "true");
  expect(screen.getByRole("tab", { name: "Alpha" })).toHaveAttribute("aria-selected", "false");
  expect(screen.getByRole("tabpanel", { name: "Beta" })).toHaveTextContent("Panel b");
});

it("puts only the selected tab in the tab order", () => {
  render(<Harness start="b" />);
  expect(screen.getByRole("tab", { name: "Beta" })).toHaveAttribute("tabindex", "0");
  expect(screen.getByRole("tab", { name: "Alpha" })).toHaveAttribute("tabindex", "-1");
  expect(screen.getByRole("tab", { name: "Gamma" })).toHaveAttribute("tabindex", "-1");
});

it("reports a click", async () => {
  const onChange = vi.fn();
  render(<Harness onChange={onChange} />);
  await userEvent.click(screen.getByRole("tab", { name: "Gamma" }));
  expect(onChange).toHaveBeenCalledWith("c");
  expect(screen.getByRole("tabpanel", { name: "Gamma" })).toHaveTextContent("Panel c");
});

it("moves and selects with the arrows, wrapping round", async () => {
  render(<Harness />);
  await userEvent.tab();
  expect(screen.getByRole("tab", { name: "Alpha" })).toHaveFocus();
  await userEvent.keyboard("{ArrowRight}");
  expect(screen.getByRole("tab", { name: "Beta" })).toHaveFocus();
  expect(screen.getByRole("tab", { name: "Beta" })).toHaveAttribute("aria-selected", "true");
  expect(screen.getByRole("tabpanel")).toHaveTextContent("Panel b");
  await userEvent.keyboard("{ArrowLeft}{ArrowLeft}");
  expect(screen.getByRole("tab", { name: "Gamma" })).toHaveFocus();
  expect(screen.getByRole("tab", { name: "Gamma" })).toHaveAttribute("aria-selected", "true");
  await userEvent.keyboard("{ArrowRight}");
  expect(screen.getByRole("tab", { name: "Alpha" })).toHaveFocus();
  expect(screen.getByRole("tabpanel")).toHaveTextContent("Panel a");
});

it("goes to the first and last with Home and End", async () => {
  render(<Harness start="b" />);
  await userEvent.tab();
  await userEvent.keyboard("{End}");
  expect(screen.getByRole("tab", { name: "Gamma" })).toHaveFocus();
  expect(screen.getByRole("tab", { name: "Gamma" })).toHaveAttribute("aria-selected", "true");
  await userEvent.keyboard("{Home}");
  expect(screen.getByRole("tab", { name: "Alpha" })).toHaveFocus();
  expect(screen.getByRole("tab", { name: "Alpha" })).toHaveAttribute("aria-selected", "true");
});
