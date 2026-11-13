import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { Select } from "./Select";
import { Field } from "./Field";

it("lists the options, with the placeholder first, and reports a choice", async () => {
  const onChange = vi.fn();
  render(
    <Select
      aria-label="Effort"
      options={[{ value: "low" }, { value: "high", label: "High" }]}
      value=""
      placeholder="Default"
      onChange={onChange}
      disabled={false}
    />,
  );
  expect(screen.getAllByRole("option").map((o) => o.textContent)).toEqual([
    "Default",
    "low",
    "High",
  ]);
  await userEvent.selectOptions(
    screen.getByRole("combobox", { name: "Effort" }),
    "high",
  );
  expect(onChange).toHaveBeenCalledWith("high");
});

it("passes disabled through", () => {
  render(
    <Select
      aria-label="X"
      options={[]}
      value=""
      onChange={() => {}}
      disabled
    />,
  );
  expect(screen.getByRole("combobox", { name: "X" })).toBeDisabled();
});

it("Field labels its control and shows the hint", () => {
  render(
    <Field label="Branch" hint="Exact name." hintId="h">
      <input aria-describedby="h" />
    </Field>,
  );
  expect(
    screen.getByRole("textbox", { name: "Branch" }),
  ).toHaveAccessibleDescription("Exact name.");
});
