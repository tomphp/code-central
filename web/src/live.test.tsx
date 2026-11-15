// The <Live> provider and its read hooks, over a WASM Core world: live
// reads follow change notices and reconnects through the one feed, Usage
// reads load only when their arguments change, and a hook outside <Live>
// says so.
import { act, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { newWorld, type World } from "./api/core/core";
import { scenario } from "./api/scenario";
import type { SessionUsageFilter, UsageDays } from "./api/types";
import { useCIOverview, useInbox, useOptions, useSessionUsage, useSpec, useTicket, useUsage } from "./live";
import { renderLive } from "./worldTesting";

async function acme() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const id = await s.ticket("acme", { title: "Fix login" });
  return { w, s, id };
}

function TicketTitle({ id }: { id: string }) {
  const t = useTicket("acme", id);
  return <h1>{t.data?.title ?? "Loading"}</h1>;
}

// settled lets the provider's effects run, so its feed is open before a
// change is made.
async function settled() {
  await act(async () => {});
}

describe("the live read hooks", () => {
  it("show a change made through the api once its notice arrives", async () => {
    const { w, id } = await acme();
    renderLive(w, <TicketTitle id={id} />);
    await screen.findByRole("heading", { name: "Fix login" });
    await settled();

    await act(() => w.api.act("acme", id, "edit_ticket", { title: "Fix logout" }));
    expect(await screen.findByRole("heading", { name: "Fix logout" })).toBeInTheDocument();
  });

  it("bring in a change made while the feed was down once it reconnects", async () => {
    const { w, id } = await acme();
    renderLive(w, <TicketTitle id={id} />);
    await screen.findByRole("heading", { name: "Fix login" });
    await settled();

    w.dropStreams();
    await w.api.act("acme", id, "edit_ticket", { title: "Fix logout" });
    expect(await screen.findByRole("heading", { name: "Fix logout" })).toBeInTheDocument();
  });

  it("read nothing until given what they read", async () => {
    const { w } = await acme();
    function Pane() {
      const reads = [useTicket("acme", undefined), useSpec(undefined, "acme-1"), useCIOverview(undefined), useUsage(undefined), useSessionUsage(undefined)];
      return <p>{reads.some((r) => r.data) ? "Read" : "Nothing"}</p>;
    }
    renderLive(w, <Pane />);
    await settled();
    expect(screen.getByText("Nothing")).toBeInTheDocument();
    expect(w.calls).toEqual([]);
  });

  it("read the CI overview again on each notice", async () => {
    const { w, id } = await acme();
    function CI() {
      const ci = useCIOverview({ project: "acme" });
      return <p>{ci.data ? "CI read" : "Loading"}</p>;
    }
    renderLive(w, <CI />);
    await screen.findByText("CI read");
    await settled();
    const reads = () => w.calls.filter((c) => c === "ciOverview").length;
    expect(reads()).toBe(1);

    await act(() => w.api.act("acme", id, "edit_ticket", { title: "Fix logout" }));
    await waitFor(() => expect(reads()).toBe(2));
  });

  it("open exactly one subscription however many read", async () => {
    const { w, id } = await acme();
    function Readers() {
      useInbox();
      useOptions();
      useTicket("acme", id);
      return <TicketTitle id={id} />;
    }
    renderLive(w, <Readers />);
    await screen.findByRole("heading", { name: "Fix login" });
    await settled();
    expect(w.subscriptions()).toBe(1);
  });

  it("throw a clear error outside <Live>", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    expect(() => render(<TicketTitle id="acme-1" />)).toThrow(/useTicket.*<Live/);
  });
});

describe("the Usage reads", () => {
  function Usage({ days }: { days: UsageDays }) {
    const u = useUsage({ project: "acme", days });
    return <p>{u.data ? `Usage over ${days} days` : "Loading"}</p>;
  }

  function Session({ session }: { session: SessionUsageFilter }) {
    const u = useSessionUsage(session);
    return <p>{u.data ? `Session ${session.session}` : "Loading"}</p>;
  }

  const reads = (w: World, op: string) => w.calls.filter((c) => c === op).length;

  it("load when opened and when their arguments change, never on a notice", async () => {
    const { w, id } = await acme();
    const { rerender } = renderLive(
      w,
      <>
        <Usage days={7} />
        <Session session={{ project: "acme", session: "s-1" }} />
      </>,
    );
    await screen.findByText("Usage over 7 days");
    await screen.findByText("Session s-1");
    await settled();
    expect([reads(w, "usage"), reads(w, "sessionUsage")]).toEqual([1, 1]);

    // A notice and a reconnect, read by a live hook beside them.
    rerender(
      <>
        <Usage days={7} />
        <Session session={{ project: "acme", session: "s-1" }} />
        <TicketTitle id={id} />
      </>,
    );
    await screen.findByRole("heading", { name: "Fix login" });
    await act(() => w.api.act("acme", id, "edit_ticket", { title: "Fix logout" }));
    await screen.findByRole("heading", { name: "Fix logout" });
    // A change made while the feed is down shows only once it reconnects.
    w.dropStreams();
    await w.api.act("acme", id, "edit_ticket", { title: "Fix it all" });
    await screen.findByRole("heading", { name: "Fix it all" });
    expect([reads(w, "usage"), reads(w, "sessionUsage")]).toEqual([1, 1]);

    rerender(
      <>
        <Usage days={30} />
        <Session session={{ project: "acme", session: "s-2" }} />
      </>,
    );
    await screen.findByText("Usage over 30 days");
    await screen.findByText("Session s-2");
    expect([reads(w, "usage"), reads(w, "sessionUsage")]).toEqual([2, 2]);
  });
});
