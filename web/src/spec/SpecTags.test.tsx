// The Spec pane's Tags (Spec code-central-1089): the Spec's Tags, each
// editable and removable, a form that adds one, and core's refusal shown
// beside the Tag it refused, as a `branch` Tag's checks give it. On a WASM
// Core world.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { renderLive, scenario, settle } from "../worldTesting";
import { SpecPane } from "./SpecPane";

async function seeded() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const id = await s.spec("acme", { title: "Sign-in" });
  return { w, s, id };
}

// renderPane renders the pane on the Spec as read now; reread renders it
// again on a fresh read, as the App does after a change.
async function renderPane(w: World, id: string, archived?: boolean) {
  const pane = async () => <SpecPane spec={await w.api.getSpec("acme", id)} archived={archived} now={w.now()} select={() => {}} />;
  const r = await renderLive(w, await pane());
  return { ...r, reread: async () => r.rerender(await pane()) };
}

const tags = () => screen.getByRole("region", { name: "Tags" });
const row = (key: string) => within(tags()).getByRole("listitem", { name: key });
const addForm = () => within(tags()).getByRole("form", { name: "Add a Tag" });
const tagsOf = async (w: World, id: string) => (await w.api.getSpec("acme", id)).tags;

async function add(w: World, key: string, value: string) {
  await userEvent.type(within(addForm()).getByLabelText("Tag key"), key);
  await userEvent.type(within(addForm()).getByLabelText("Tag value"), value);
  await userEvent.click(within(addForm()).getByRole("button", { name: "Add" }));
  await settle(w);
}

async function edit(w: World, key: string, value: string) {
  const input = within(row(key)).getByLabelText(`${key} Tag value`);
  await userEvent.clear(input);
  await userEvent.type(input, value);
  await userEvent.click(within(row(key)).getByRole("button", { name: `Save ${key}` }));
  await settle(w);
}

async function remove(w: World, key: string) {
  await userEvent.click(within(row(key)).getByRole("button", { name: `Remove ${key}` }));
  await settle(w);
}

describe("the Spec pane's Tags", () => {
  it("says there are none on a Spec without Tags", async () => {
    const { w, id } = await seeded();
    await renderPane(w, id);
    expect(within(tags()).getByText("No Tags.")).toBeInTheDocument();
  });

  it("adds a branch Tag, then edits and removes it", async () => {
    const { w, id } = await seeded();
    const { reread } = await renderPane(w, id);
    await add(w, "branch", "feat/4711");
    expect(await tagsOf(w, id)).toEqual({ branch: "feat/4711" });

    await reread();
    expect(within(row("branch")).getByLabelText("branch Tag value")).toHaveValue("feat/4711");
    await edit(w, "branch", "feat/4712");
    expect(await tagsOf(w, id)).toEqual({ branch: "feat/4712" });

    await reread();
    await remove(w, "branch");
    expect(await tagsOf(w, id)).toEqual({});
  });

  it("shows a refused branch name beside the new Tag, and keeps the draft", async () => {
    const { w, id } = await seeded();
    await renderPane(w, id);
    await add(w, "branch", "spec/x");
    expect(within(addForm()).getByRole("alert")).toHaveTextContent(/spec\//);
    expect(within(addForm()).getByLabelText("Tag value")).toHaveValue("spec/x");
    expect(await tagsOf(w, id)).toEqual({});
  });

  it("shows a clash naming the other Spec beside the Tag", async () => {
    const { w, s, id } = await seeded();
    const other = await s.spec("acme", { title: "Export" });
    await w.api.setTags("acme", other, { branch: "feat/4711" });
    await w.api.setTags("acme", id, { branch: "feat/1" });
    await renderPane(w, id);
    await edit(w, "branch", "feat/4711");
    expect(within(row("branch")).getByRole("alert")).toHaveTextContent(other);
  });

  it("shows the lock once an implementation Ticket has been claimed", async () => {
    const { w, s, id } = await seeded();
    await w.api.setTags("acme", id, { branch: "feat/4711" });
    await s.ticket("acme", { title: "Add a page", spec: id, state: "In Progress" });
    await renderPane(w, id);
    await remove(w, "branch");
    expect(within(row("branch")).getByRole("alert")).toHaveTextContent(/already exists/);
    expect(await tagsOf(w, id)).toEqual({ branch: "feat/4711" });
  });

  it("offers no write while the Project is Archived", async () => {
    const { w, id } = await seeded();
    await w.api.setTags("acme", id, { area: "web" });
    await renderPane(w, id, true);
    expect(within(row("area")).getByLabelText("area Tag value")).toBeDisabled();
    expect(within(row("area")).getByRole("button", { name: "Remove area" })).toBeDisabled();
    expect(within(addForm()).getByLabelText("Tag key")).toBeDisabled();
  });
});
