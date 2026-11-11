// The Factory Status dot in the left pane and the Factories section of
// Project Options, against FakeApi (code-central-324).
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const beat = { factory: "code-central-factory", host: "laptop", interval_seconds: 30 } as const;

describe("Factory Status dot", () => {
  it.each([
    ["online", "Online", "online"],
    ["draining", "Draining", "draining"],
    ["stopping", "Offline", "offline"],
  ] as const)("is %s -> %s, named for it", async (state, label, cls) => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.reportHeartbeat("acme", { ...beat, state });
    renderApp(fake);
    const dot = await within(nav()).findByRole("img", { name: `Factory Status: ${label}` });
    expect(dot).toHaveClass(cls);
    expect(dot).toHaveAttribute("tabindex", "0");
  });

  it("is Offline when no Factory has ever reported", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);
    expect(await within(nav()).findByRole("img", { name: "Factory Status: Offline" })).toHaveClass("offline");
  });

  it("is not shown on an Archived Project", async () => {
    const fake = new FakeApi();
    fake.addProject("old", "Old", { archived: true });
    fake.addProject("acme", "Acme");
    renderApp(fake);
    await within(nav()).findByRole("img", { name: "Factory Status: Offline" });
    expect(within(nav()).getAllByRole("img", { name: /Factory Status/ })).toHaveLength(1);
  });

  it("lists the Factories in a tooltip on hover and on focus", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.reportHeartbeat("acme", { ...beat, state: "online", note: "1 of 3 turns running" });
    renderApp(fake);
    const dot = await within(nav()).findByRole("img", { name: "Factory Status: Online" });
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();

    await userEvent.hover(dot);
    expect(screen.getByRole("tooltip")).toHaveTextContent("code-central-factory on laptop: Online, last Heartbeat just now (1 of 3 turns running)");
    await userEvent.unhover(dot);
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();

    await userEvent.tab();
    dot.focus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("code-central-factory on laptop");
  });

  it("says no Factory has sent a Heartbeat when there are none", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);
    await userEvent.hover(await within(nav()).findByRole("img", { name: "Factory Status: Offline" }));
    expect(screen.getByRole("tooltip")).toHaveTextContent("No Factory has sent a Heartbeat since Code Central started.");
  });
});

describe("Project Options Factories section", () => {
  it("lists the Factories, a stopped one keeping its note", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.reportHeartbeat("acme", { ...beat, state: "stopping", note: "stopped: drained" });
    renderApp(fake, "/?view=project-options&project=acme");
    const group = await screen.findByRole("group", { name: "Factories" });
    expect(group).toHaveTextContent("code-central-factory on laptop: Offline, last Heartbeat just now (stopped: drained)");
  });

  it("shows the empty line with links to the guides", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake, "/?view=project-options&project=acme");
    const group = await screen.findByRole("group", { name: "Factories" });
    expect(group).toHaveTextContent("No Factory has sent a Heartbeat since Code Central started.");
    expect(within(group).getByRole("link", { name: "running a Factory" })).toHaveAttribute("href", "docs/running-the-factory.md");
    expect(within(group).getByRole("link", { name: "writing a Factory" })).toHaveAttribute("href", "docs/writing-a-factory.md");
  });
});
