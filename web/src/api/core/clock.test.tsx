// The App on a WASM Core world reads the world's clock, so advance moves
// Core and the screen together.
import { render, screen, within } from "@testing-library/react";
import { expect, it } from "vitest";
import { App } from "../../App";
import { scenario } from "../scenario";
import { newWorld } from "./core";

it("shows how long a Ticket has waited by the world's clock", async () => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const id = await s.ticket("acme", { title: "Pick a colour", state: "Needs Answer" });
  w.advance(3 * 60 * 60_000);

  window.history.replaceState(null, "", "/?view=inbox");
  render(<App api={w.api} now={w.now} />);
  const row = await within(await screen.findByRole("main", { name: "Tickets" })).findByRole("link", { name: new RegExp(id) });
  expect(row).toHaveTextContent("Pick a colour");
  expect(row).toHaveTextContent("3h");
});
