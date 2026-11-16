// The App on a WASM Core world reads the world's clock, so advance moves
// Core and the screen together.
import { screen, within } from "@testing-library/react";
import { expect, it } from "vitest";
import { renderApp, scenario } from "../../worldTesting";
import { newWorld } from "./core";

it("shows how long a Ticket has waited by the world's clock", async () => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  const id = await s.ticket("acme", { title: "Pick a colour", state: "Needs Answer" });
  w.advance(3 * 60 * 60_000);

  await renderApp(w, "/?view=inbox");
  const row = within(screen.getByRole("main", { name: "Tickets" })).getByRole("link", { name: new RegExp(id) });
  expect(row).toHaveTextContent("Pick a colour");
  expect(row).toHaveTextContent("3h");
});
