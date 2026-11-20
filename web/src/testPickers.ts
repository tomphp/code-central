// Reading the model and effort pickers, and driving the Project picker, in tests.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// options is what a select offers, as shown.
export const options = (select: HTMLElement) => within(select).getAllByRole("option").map((o) => o.textContent);

// suggested is what a model picker's list suggests, none without a list.
export const suggested = (input: HTMLElement) => {
  const list = input.getAttribute("list");
  return list ? Array.from(document.getElementById(list)?.querySelectorAll("option") ?? [], (o) => o.value) : [];
};

// projectPicker is the Project picker's button in a view, named "Projects" and the chosen entry.
export const projectPicker = (view: HTMLElement) => within(view).getByRole("button", { name: /^Projects/ });

// openProjectPicker opens the menu and returns its list box.
export async function openProjectPicker(view: HTMLElement) {
  await userEvent.click(projectPicker(view));
  return screen.getByRole("listbox", { name: "Projects" });
}

// projectChoices is the Project picker's options as shown, by name.
export const projectChoices = (listbox: HTMLElement) => options(listbox).map((n) => n?.replace("✓", ""));

// chooseProject opens the Project picker in a view and clicks the option shown as name.
export async function chooseProject(view: HTMLElement, name: string) {
  const listbox = await openProjectPicker(view);
  const i = projectChoices(listbox).indexOf(name);
  if (i < 0) throw new Error(`no Project option ${name}`);
  await userEvent.click(within(listbox).getAllByRole("option")[i]);
}
