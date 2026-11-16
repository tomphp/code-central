// MapBody takes a Map and the outcome of its actions as props: these tests
// render it on a WASM Core world's Map, with the callbacks as spies.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld } from "../api/core/core";
import { scenario } from "../api/scenario";
import type { MapView, SpecDetail } from "../api/types";
import { renderLive } from "../worldTesting";
import { MapBody, type MapBodyProps } from "./MapPane";

async function map(populate = false) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const { spec } = await w.api.submitMap("acme", "Offline mode", true);
  if (populate) await w.api.editSpec("acme", spec.id, { body: "## Destination\n\nWork on a plane.\n\n## Not yet specified\n\nSync." });
  const read = await w.api.getSpec("acme", spec.id);
  return { w, spec: { ...read, map: read.map! } as SpecDetail & { map: MapView } };
}

async function body(props: Partial<MapBodyProps> = {}, populate = false) {
  const { w, spec } = await map(populate);
  const start = vi.fn();
  const setGuided = vi.fn();
  await renderLive(
    w,
    <MapBody spec={spec} now={w.now()} busy={false} select={() => {}} specLink={() => ({ href: "#", onClick: () => {} })} written={() => {}} start={start} setGuided={setGuided} {...props} />,
  );
  return { start, setGuided, spec };
}

describe("MapBody", () => {
  it("says there is no Destination when the Map has none", async () => {
    await body();
    expect(screen.getByLabelText("Destination")).toHaveTextContent("No Destination yet.");
  });

  it("shows the Destination and opens the fog text on click", async () => {
    await body({}, true);
    expect(screen.getByLabelText("Destination")).toHaveTextContent("Work on a plane.");
    expect(screen.queryByRole("region", { name: "Not yet specified" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Not yet specified" }));
    expect(screen.getByRole("region", { name: "Not yet specified" })).toHaveTextContent("Sync.");
  });

  it("calls start with a frontier Ticket and setGuided with the toggle", async () => {
    const { start, setGuided } = await body();
    await userEvent.click(screen.getByRole("button", { name: /^Start / }));
    expect(start).toHaveBeenCalledWith(expect.objectContaining({ state: "Draft" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Guided" }));
    expect(setGuided).toHaveBeenCalledWith(true);
  });

  it("disables Start and Guided while busy", async () => {
    await body({ busy: true });
    expect(screen.getByRole("button", { name: /^Start / })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: "Guided" })).toBeDisabled();
  });

  it("shows the last action's error", async () => {
    await body({ error: "boom" });
    expect(within(screen.getByRole("region", { name: "Map" })).getByText("boom")).toBeInTheDocument();
  });
});
