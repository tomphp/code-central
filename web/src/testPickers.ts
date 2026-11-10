// Reading the model and effort pickers in tests.
import { within } from "@testing-library/react";

// options is what a select offers, as shown.
export const options = (select: HTMLElement) => within(select).getAllByRole("option").map((o) => o.textContent);

// suggested is what a model picker's list suggests, none without a list.
export const suggested = (input: HTMLElement) => {
  const list = input.getAttribute("list");
  return list ? Array.from(document.getElementById(list)?.querySelectorAll("option") ?? [], (o) => o.value) : [];
};
