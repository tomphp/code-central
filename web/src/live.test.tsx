// The <Live> provider and its read hooks, over a WASM Core world: live
// reads follow change notices and reconnects through the one feed, Usage
// reads load only when their arguments change, and a hook outside <Live>
// says so. Each settles the world (ADR 0044) before it asserts, so a
// change made once the feed is open always reaches it; settle itself is
// tested here too, with React.
import { render, screen } from "@testing-library/react";
import { useEffect, useState } from "react";
import { describe, expect, it, onTestFinished, vi } from "vitest";
import { newWorld, type World } from "./api/core/core";
import { scenario } from "./api/scenario";
import type { SessionUsageFilter, UsageDays } from "./api/types";
import { useCIOverview, useInbox, useOptions, useSessionUsage, useSpec, useTicket, useUsage } from "./live";
import { renderLive, settle } from "./worldTesting";

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

describe("the live read hooks", () => {
  it("show a change made through the api once its notice arrives", async () => {
    const { w, id } = await acme();
    await renderLive(w, <TicketTitle id={id} />);
    await settle(w);
    expect(screen.getByRole("heading", { name: "Fix login" })).toBeInTheDocument();

    await w.api.act("acme", id, "edit_ticket", { title: "Fix logout" });
    await settle(w);
    expect(screen.getByRole("heading", { name: "Fix logout" })).toBeInTheDocument();
  });

  it("bring in a change made while the feed was down once it reconnects", async () => {
    const { w, id } = await acme();
    await renderLive(w, <TicketTitle id={id} />);
    await settle(w);

    w.dropStreams();
    await w.api.act("acme", id, "edit_ticket", { title: "Fix logout" });
    await settle(w);
    expect(screen.getByRole("heading", { name: "Fix logout" })).toBeInTheDocument();
  });

  it("read nothing until given what they read", async () => {
    const { w } = await acme();
    function Pane() {
      const reads = [useTicket("acme", undefined), useSpec(undefined, "acme-1"), useCIOverview(undefined), useUsage(undefined), useSessionUsage(undefined)];
      return <p>{reads.some((r) => r.data) ? "Read" : "Nothing"}</p>;
    }
    await renderLive(w, <Pane />);
    await settle(w);
    expect(screen.getByText("Nothing")).toBeInTheDocument();
    expect(w.calls).toEqual([]);
  });

  it("read the CI overview again on each notice", async () => {
    const { w, id } = await acme();
    function CI() {
      const ci = useCIOverview({ project: "acme" });
      return <p>{ci.data ? "CI read" : "Loading"}</p>;
    }
    await renderLive(w, <CI />);
    await settle(w);
    expect(screen.getByText("CI read")).toBeInTheDocument();
    const reads = () => w.calls.filter((c) => c === "ciOverview").length;
    expect(reads()).toBe(1);

    await w.api.act("acme", id, "edit_ticket", { title: "Fix logout" });
    await settle(w);
    expect(reads()).toBe(2);
  });

  it("open exactly one subscription however many read", async () => {
    const { w, id } = await acme();
    function Readers() {
      useInbox();
      useOptions();
      useTicket("acme", id);
      return <TicketTitle id={id} />;
    }
    await renderLive(w, <Readers />);
    await settle(w);
    expect(screen.getByRole("heading", { name: "Fix login" })).toBeInTheDocument();
    expect(w.subscriptions()).toBe(1);
  });

  it("throw a clear error outside <Live>", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    expect(() => render(<TicketTitle id="acme-1" />)).toThrow(/useTicket.*<Live/);
  });
});

describe("settling a world under React", () => {
  it("renders what the world brought while the test fakes timers", async () => {
    vi.useFakeTimers();
    onTestFinished(() => void vi.useRealTimers());
    const { w, id } = await acme();
    await renderLive(w, <TicketTitle id={id} />);
    await settle(w);
    expect(screen.getByRole("heading", { name: "Fix login" })).toBeInTheDocument();
  });

  it("fails clearly, rather than hang, while the UI keeps fetching", async () => {
    const w = await newWorld();
    function Restless() {
      const [reads, setReads] = useState(0);
      useEffect(() => {
        w.api.listProjects().then(
          () => setReads(reads + 1),
          () => {},
        );
      }, [reads]);
      return <p>Read {reads} times</p>;
    }
    render(<Restless />);
    await expect(settle(w)).rejects.toThrow(/the UI keeps fetching/);
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
    const { rerender } = await renderLive(
      w,
      <>
        <Usage days={7} />
        <Session session={{ project: "acme", session: "s-1" }} />
      </>,
    );
    await settle(w);
    expect(screen.getByText("Usage over 7 days")).toBeInTheDocument();
    expect(screen.getByText("Session s-1")).toBeInTheDocument();
    expect([reads(w, "usage"), reads(w, "sessionUsage")]).toEqual([1, 1]);

    // A notice and a reconnect, read by a live hook beside them.
    rerender(
      <>
        <Usage days={7} />
        <Session session={{ project: "acme", session: "s-1" }} />
        <TicketTitle id={id} />
      </>,
    );
    await settle(w);
    expect(screen.getByRole("heading", { name: "Fix login" })).toBeInTheDocument();
    await w.api.act("acme", id, "edit_ticket", { title: "Fix logout" });
    await settle(w);
    expect(screen.getByRole("heading", { name: "Fix logout" })).toBeInTheDocument();
    // A change made while the feed is down shows only once it reconnects.
    w.dropStreams();
    await w.api.act("acme", id, "edit_ticket", { title: "Fix it all" });
    await settle(w);
    expect(screen.getByRole("heading", { name: "Fix it all" })).toBeInTheDocument();
    expect([reads(w, "usage"), reads(w, "sessionUsage")]).toEqual([1, 1]);

    rerender(
      <>
        <Usage days={30} />
        <Session session={{ project: "acme", session: "s-2" }} />
      </>,
    );
    await settle(w);
    expect(screen.getByText("Usage over 30 days")).toBeInTheDocument();
    expect(screen.getByText("Session s-2")).toBeInTheDocument();
    expect([reads(w, "usage"), reads(w, "sessionUsage")]).toEqual([2, 2]);
  });
});
