// Activity view tests: the real App on a WASM Core world, at ?view=activity.
// They check what the human sees and does: opening the Activity Dashboard
// from the Inbox's Admin section on the sidebar's Project or All Projects,
// its Projects picker and Back, and that it shows whatever needs the human.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { chooseProject, projectPicker } from "../testPickers";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { loadedDashboard } from "../focus/worldTesting";
import { renderApp, scenario, settle } from "../worldTesting";

async function projects() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  await s.ticket("acme", { title: "Login form", state: "Ready" });
  await s.ticket("acme", { title: "Sign-up page", state: "In Progress" });
  await s.ticket("beta", { title: "Beta export", state: "In Progress" });
  return { w, s };
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });

describe("opening the Activity view from the Inbox", () => {
  it("links first in the View section, before Board and Needs Me, on the sidebar's Project", async () => {
    await renderApp((await projects()).w,"/?view=inbox&project=acme&state=Accepted");
    const link = within(nav()).getByRole("link", { name: "Activity" });
    const view = within(nav()).getByRole("heading", { level: 2, name: "View" });
    const board = within(nav()).getByRole("link", { name: /^Board/ });
    const needsMe = within(nav()).getByRole("button", { name: /^Needs Me/ });
    expect(view.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link.compareDocumentPosition(board) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link.compareDocumentPosition(needsMe) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link).toHaveAttribute("href", "?view=activity&project=acme&back=view%3Dinbox%26project%3Dacme%26state%3DAccepted");
    expect(link).not.toHaveTextContent(/\d/);
  });

  it("links on All Projects when the sidebar has none chosen", async () => {
    await renderApp((await projects()).w,"/?view=inbox");
    expect(within(nav()).getByRole("link", { name: "Activity" })).toHaveAttribute("href", "?view=activity");
  });
});

const activity = () => screen.getByRole("main", { name: "Activity" });
const dashboard = (w: World) => loadedDashboard(w, activity());

describe("the Activity view", () => {
  it("shows the Activity Dashboard of its Project, whatever needs the human and whatever Focus Mode shows when nothing does", async () => {
    const { w, s } = await projects();
    await s.ticket("acme", { title: "Broken build", state: "Failed" });
    await w.api.setOptions({ focus_clear: "all_clear" });
    await renderApp(w, "/?view=activity&project=acme");
    const d = await dashboard(w);
    expect(within(within(d).getByRole("list", { name: "In Progress" })).getAllByRole("listitem")).toHaveLength(1);
    expect(d).toHaveTextContent("Sign-up page");
    expect(d).not.toHaveTextContent("Beta export");
  });

  it("opens from the link on the sidebar's Project and Back returns to the Inbox as it was", async () => {
    await renderApp((await projects()).w,"/?view=inbox&project=acme&state=Accepted");
    await userEvent.click(within(nav()).getByRole("link", { name: "Activity" }));
    expect(projectPicker(activity())).toHaveTextContent("Acme");

    await userEvent.click(within(activity()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox&project=acme&state=Accepted");
  });

  it("changes scope with the Projects picker, keeping the address", async () => {
    const { w } = await projects();
    await renderApp(w, "/?view=activity");
    await chooseProject(activity(), "Beta");
    expect(window.location.search).toBe("?view=activity&project=beta");
    const d = await dashboard(w);
    expect(d).toHaveTextContent("Beta export");
    expect(d).not.toHaveTextContent("Sign-up page");
  });

  it("opens an In Progress Ticket in the Inbox", async () => {
    const { w } = await projects();
    await renderApp(w, "/?view=activity&project=acme");
    await userEvent.click(within(await dashboard(w)).getByRole("link", { name: /Sign-up page/ }));
    expect(window.location.search).toContain("view=inbox");
    expect(window.location.search).toContain("ticket=");
  });

  it("returns from the CI view its CI Running block opens to the Activity view, on the same Project", async () => {
    const { w, s } = await projects();
    await s.trackCI("acme");
    await renderApp(w, "/?view=activity&project=acme");
    const d = await dashboard(w);
    // The block remounts when the count arrives, so click its link only after that.
    expect(within(within(d).getByRole("group", { name: "CI Running" })).getByTestId("count")).toHaveTextContent("0");
    await userEvent.click(within(d).getByRole("link", { name: /CI Running/ }));
    expect(window.location.search).toContain("view=ci");

    await userEvent.click(screen.getByRole("button", { name: "Back to Activity" }));
    await settle(w);
    expect(window.location.search).toBe("?view=activity&project=acme");
    expect(await dashboard(w)).toBeInTheDocument();
  });
});
