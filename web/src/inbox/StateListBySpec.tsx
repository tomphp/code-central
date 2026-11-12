// A State list grouped By Spec: one card per Spec with Tickets in the list,
// opening on those Tickets. Which cards are open lives here, on the page
// only.
import { useEffect, useState } from "react";
import type { MouseEvent } from "react";
import type { NeedsMeReason, State } from "../api/types";
import type { ProjectSpec, ProjectTicket } from "../live";
import { groupBySpec } from "./groupBySpec";
import { StateRow, type InboxRowLinkFactory } from "./StateRow";
import { ShortId } from "./ShortId";

const slug = (s: State) => s.toLowerCase().replace(/\s+/g, "-");

export function StateListBySpec({
  state,
  tickets,
  specs,
  reasons,
  now,
  link,
  specLink,
  selectedTicket,
  selectedSpec,
}: {
  state: State;
  tickets: ProjectTicket[];
  specs: Map<string, ProjectSpec>;
  reasons: Map<string, NeedsMeReason>;
  now: Date;
  link: InboxRowLinkFactory;
  specLink(id: string): { href: string; onClick(e: MouseEvent): void };
  selectedTicket?: string;
  selectedSpec?: string;
}) {
  const cards = groupBySpec(tickets, specs, reasons);
  const selectedCard = tickets.find((t) => t.id === selectedTicket)?.spec;
  const [open, setOpen] = useState(() => new Set(selectedCard ? [selectedCard] : []));
  // The card holding the selected Ticket opens on its own, once per
  // selection, so the human can still close it.
  useEffect(() => {
    if (selectedCard) setOpen((o) => new Set(o).add(selectedCard));
  }, [selectedTicket, selectedCard]);
  const toggle = (id: string) =>
    setOpen((o) => {
      const next = new Set(o);
      if (!next.delete(id)) next.add(id);
      return next;
    });

  if (cards.length === 0) return <p className="empty">No Tickets {state}.</p>;
  return (
    <div className="bs-cards">
      {cards.map((c) => {
        const isOpen = open.has(c.id);
        const l = specLink(c.id);
        return (
          <section key={c.id} className={`bs-card${isOpen ? " open" : ""}${c.needsYou ? " needs" : ""}`} aria-label={c.title} data-needs={c.needsYou || undefined}>
            <div className="bs-head">
              <button className="bs-chev" aria-expanded={isOpen} aria-label={c.title} onClick={() => toggle(c.id)}>
                {isOpen ? "▾" : "▸"}
              </button>
              <div className="bs-head-main">
                <div className="bs-head-top">
                  <a className="bs-title" href={l.href} onClick={l.onClick} aria-current={selectedSpec === c.id ? "true" : undefined}>
                    {c.title}
                  </a>
                  {c.needsYou && <span className="dot needs-you" role="img" aria-label="Needs you" />}
                </div>
                <div className="bs-head-sub">
                  <span className={`bs-kind bs-kind-${c.kind.toLowerCase()}`}>{c.kind}</span>
                  <ShortId id={c.id} />
                  <span className="bs-count">
                    {c.total > 0 ? `${c.tickets.length} of ${c.total}` : c.tickets.length}
                  </span>
                </div>
                <span className="bs-chips">
                  {c.counts.map((x) => (
                    <span key={x.state} className={`bs-chip st-${slug(x.state)}${x.state === state ? " here" : ""}`} aria-current={x.state === state ? "true" : undefined}>
                      {x.state} {x.count}
                    </span>
                  ))}
                </span>
              </div>
            </div>
            {isOpen && (
              <div className="bs-rows">
                {c.tickets.map((t) => (
                  <StateRow key={t.id} t={t} needsYou={reasons.has(t.id)} now={now} link={link} stage />
                ))}
              </div>
            )}
          </section>
        );
      })}
    </div>
  );
}
