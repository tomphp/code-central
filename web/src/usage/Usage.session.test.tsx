// Usage view tests: the Session popup.
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { Api } from "../api/api";
import { scenario, settle } from "../worldTesting";
import { projects, world, renderApp, usage, scope, back, openTab, claimed, response } from "./Usage.testutil";

describe("the Session popup", () => {
  async function withUsage() {
    const { w, s } = await projects();
    await claimed(s, "acme", "Build it", "sess-impl", [
      response(w, 1, "implementation", 40_000),
      response(w, 2, "implementation", 123_456),
      response(w, 3, "review", 21_000, "toolu_review"),
      response(w, 4, "implementation", 98_765),
    ]);
    return w;
  }

  const sessionsTable = () => within(usage()).getByRole("table", { name: "Recent sessions" });
  const popup = () => screen.getByRole("dialog", { name: /^Session / });
  const noPopup = () => expect(screen.queryByRole("dialog")).toBeNull();
  const chart = () => within(popup()).getByRole("img", { name: "Context Size per response" });
  const points = () => within(chart()).getAllByRole("button");
  const search = () => new URLSearchParams(window.location.search);

  it("opens a session from the recent sessions as a popup of its Context Size per response, over the table", async () => {
    await renderApp(await withUsage(), "/?view=usage");
    await openTab("Context Size");
    await settle(world);
    expect(sessionsTable()).toBeInTheDocument();
    const link = within(sessionsTable()).getByRole("link", {
      name: "sess-impl",
    });
    expect(link).toHaveAttribute("href", "?view=usage&tab=context&session_project=acme&session=sess-impl");

    await userEvent.click(link);
    expect(search().get("session")).toBe("sess-impl");
    expect(search().get("session_project")).toBe("acme");
    expect(search().get("agent")).toBeNull();
    await settle(world);
    expect(points()).toHaveLength(3);
    expect(popup()).toHaveAccessibleName("Session sess-impl");
    expect(within(popup()).getByText("3 responses · peak 123,456 tokens")).toBeInTheDocument();
    expect(points().map((p) => p.getAttribute("aria-label"))).toEqual(["Response 1: 40,000 tokens", "Response 2: 123,456 tokens", "Response 3: 98,765 tokens"]);
    // The highest point is drawn highest, the lowest lowest.
    const ys = points().map((p) => Number(p.getAttribute("cy")));
    expect(ys[1]).toBeLessThan(ys[2]);
    expect(ys[2]).toBeLessThan(ys[0]);
    // Recent sessions stays where it was, under the popup.
    expect(sessionsTable()).toBeInTheDocument();
  });

  it("opens a Subagent's popup with its agent in the address, and its parent link switches the popup in place", async () => {
    await renderApp(await withUsage(), "/?view=usage&days=30");
    await openTab("Context Size");
    await settle(world);
    expect(sessionsTable()).toBeInTheDocument();
    await userEvent.click(within(sessionsTable()).getByRole("button", { name: "1 subagent" }));
    const link = within(sessionsTable()).getByRole("link", { name: "toolu_review" });
    // The link keeps the range and tab too, for a new tab or a copied link.
    expect(link).toHaveAttribute("href", "?view=usage&days=30&tab=context&session_project=acme&session=sess-impl&agent=toolu_review");
    await userEvent.click(link);
    expect(window.location.search).toBe("?view=usage&days=30&tab=context&session_project=acme&session=sess-impl&agent=toolu_review");
    await settle(world);
    expect(points()).toHaveLength(1);
    expect(popup()).toHaveAccessibleName("Session sess-impl · toolu_review");
    expect(points()[0]).toHaveAttribute("aria-label", "Response 1: 21,000 tokens");
    // Its parent stays expanded, its row marked open.
    expect(within(sessionsTable()).getByRole("link", { name: "toolu_review" })).toHaveAttribute("aria-current", "true");

    // The popup links to the parent, whose popup is the same address without agent.
    const parent = within(popup()).getByRole("link", { name: "Parent session sess-impl" });
    expect(parent).toHaveAttribute("href", "?view=usage&days=30&tab=context&session_project=acme&session=sess-impl");
    await userEvent.click(parent);
    expect(window.location.search).toBe("?view=usage&days=30&tab=context&session_project=acme&session=sess-impl");
    await settle(world);
    expect(points()).toHaveLength(3);
    // The popup switched in place: still one, now the parent's.
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(popup()).toHaveAccessibleName("Session sess-impl");
    expect(within(popup()).queryByRole("link", { name: /Parent session/ })).toBeNull();
  });

  it("names a Subagent's popup from the read and starts its parent expanded when loaded from the address", async () => {
    const w = await withUsage();
    await scenario(w).postUsage("sess-impl", [{ ...response(w, 5, "subagent", 10_000, "toolu_a"), agent_type: "Explore", description: "Map Usage sessions" }]);
    await renderApp(w, "/?view=usage&session_project=acme&session=sess-impl&agent=toolu_a");
    expect(popup()).toHaveAccessibleName("Session sess-impl · Explore · Map Usage sessions");
    const disclosure = within(sessionsTable()).getByRole("button", { name: "2 subagents" });
    expect(disclosure).toHaveAttribute("aria-expanded", "true");
    expect(within(sessionsTable()).getByRole("link", { name: "Explore · Map Usage sessions" })).toHaveAttribute("aria-current", "true");
  });

  it("opens a session from the address over the Context Size tab, whatever tab= says", async () => {
    await renderApp(await withUsage(), "/?view=usage&tab=calls&session_project=acme&session=sess-impl");
    await settle(world);
    expect(points()).toHaveLength(3);
    expect(within(usage()).getByRole("tab", { selected: true })).toHaveTextContent("Context Size");
    expect(within(usage()).getByRole("table", { name: "Recent sessions" })).toBeInTheDocument();
  });

  it("reopens the session from the address, whatever the scope", async () => {
    const w = await withUsage();
    await renderApp(w, "/?view=usage&project=beta&session_project=acme&session=sess-impl");
    await settle(world);
    expect(points()).toHaveLength(3);
    expect(scope()).toHaveTextContent("Beta");
    expect(w.calls.filter((c) => c === "sessionUsage")).toHaveLength(1);
  });

  it("opens even when the rest of Usage can't be read", async () => {
    const w = await withUsage();
    const down: Api = {
      ...w.api,
      usage: async () => {
        throw new Error("Usage is down");
      },
    };
    await renderApp(w, "/?view=usage&session_project=acme&session=sess-impl", down);
    expect(within(usage()).getByRole("alert")).toHaveTextContent("Usage is down");
    await settle(world);
    expect(points()).toHaveLength(3);
  });

  it("shows a point's exact figures on hover or focus, with thousands separators", async () => {
    await renderApp(await withUsage(), "/?view=usage&session_project=acme&session=sess-impl");
    await settle(world);
    expect(points()).toHaveLength(3);
    expect(within(popup()).queryByRole("tooltip")).toBeNull();

    await userEvent.hover(points()[1]);
    const tip = within(popup()).getByRole("tooltip");
    expect(tip).toHaveTextContent("Response 2");
    expect(tip).toHaveTextContent("Context Size123,456 tokens");
    expect(tip).toHaveTextContent("Input4");
    expect(tip).toHaveTextContent("Cache write1,000");
    expect(tip).toHaveTextContent("Cache read122,452");
    expect(tip).toHaveTextContent("Output50");

    await userEvent.unhover(points()[1]);
    expect(within(popup()).queryByRole("tooltip")).toBeNull();

    // And on focus, from the keyboard.
    act(() => points()[2].focus());
    expect(within(popup()).getByRole("tooltip")).toHaveTextContent("Response 3");
  });

  it.each([
    ["×", () => userEvent.click(within(popup()).getByRole("button", { name: "Close" }))],
    ["Escape", () => userEvent.keyboard("{Escape}")],
    ["a click outside", () => userEvent.click(popup().parentElement!)],
  ])("closes on %s to Usage as it was: the Context Size tab, scope, range and expanded Subagents", async (_, close) => {
    await renderApp(await withUsage(), "/?view=usage&project=acme&days=30&tab=context");
    await settle(world);
    expect(sessionsTable()).toBeInTheDocument();
    const disclosure = within(sessionsTable()).getByRole("button", { name: "1 subagent" });
    await userEvent.click(disclosure);
    await userEvent.click(within(sessionsTable()).getByRole("link", { name: "toolu_review" }));
    await settle(world);
    expect(points()).toHaveLength(1);

    await close();
    noPopup();
    expect(window.location.search).toBe("?view=usage&project=acme&days=30&tab=context");
    expect(within(usage()).getByRole("tab", { selected: true })).toHaveTextContent("Context Size");
    expect(disclosure).toHaveAttribute("aria-expanded", "true");
    expect(within(sessionsTable()).getByRole("link", { name: "toolu_review" })).not.toHaveAttribute("aria-current");
    // Focus goes back to the row it was opened from.
    expect(within(sessionsTable()).getByRole("link", { name: "toolu_review" })).toHaveFocus();
    // Collapsing is the table's own state: the address gains nothing.
    await userEvent.click(disclosure);
    expect(disclosure).toHaveAttribute("aria-expanded", "false");
    expect(window.location.search).toBe("?view=usage&project=acme&days=30&tab=context");
  });

  it("closes to the Context Size tab it showed over when the address named no tab", async () => {
    await renderApp(await withUsage(), "/?view=usage&session_project=acme&session=sess-impl");
    await settle(world);
    expect(points()).toHaveLength(3);
    await userEvent.keyboard("{Escape}");
    noPopup();
    expect(window.location.search).toBe("?view=usage&tab=context");
    expect(within(usage()).getByRole("tab", { selected: true })).toHaveTextContent("Context Size");
  });

  it("closes on the browser's Back, and leaves Usage on the next", async () => {
    await renderApp(await withUsage(), "/?view=inbox&project=acme");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Usage" }));
    await openTab("Context Size");
    await settle(world);
    expect(sessionsTable()).toBeInTheDocument();
    await userEvent.click(within(sessionsTable()).getByRole("link", { name: "sess-impl" }));
    await settle(world);
    expect(points()).toHaveLength(3);

    await back();
    noPopup();
    expect(search().get("session")).toBeNull();
    expect(within(usage()).getByRole("tab", { selected: true })).toHaveTextContent("Context Size");

    await back();
    expect(within(usage()).getByRole("tab", { selected: true })).toHaveTextContent("Token usage");
    await back();
    expect(screen.getByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("returns to where Usage was opened from once the popup is closed", async () => {
    await renderApp(await withUsage(), "/?view=usage&session_project=acme&session=sess-impl&back=view%3Dfocus%26project%3Dacme");
    await settle(world);
    expect(points()).toHaveLength(3);
    await userEvent.keyboard("{Escape}");
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to Focus Mode" }));
    expect(window.location.search).toBe("?view=focus&project=acme");
  });

  it("says when a session has no responses in the days Usage keeps", async () => {
    await renderApp(await withUsage(), "/?view=usage&session_project=acme&session=sess-gone");
    expect(within(popup()).getByText("No responses for this session in the 90 days Usage keeps.")).toBeInTheDocument();
    expect(within(popup()).queryByRole("img")).toBeNull();
  });
});
