import { screen, within } from "@testing-library/react";
import { newWorld } from "../api/core/core";
import { scenario } from "../worldTesting";

// acme is a WASM Core world with the Project acme, which has no Spec.
export async function acme() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  return { w, s };
}

export const nav = () => screen.getByRole("navigation", { name: "Filters" });
export const page = () => screen.getByRole("main", { name: "Project Options" });
export const landing = () =>
  within(page()).getByRole("group", { name: "Landing Method" });
export const ci = () => within(page()).getByRole("group", { name: "CI" });
export const rules = () => within(page()).getByRole("group", { name: "CI Rules" });
export const addForm = () =>
  within(rules()).getByRole("form", { name: "Add a CI Rule" });
