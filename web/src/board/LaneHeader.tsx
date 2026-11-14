// The Lane header (in the glossary): the bar across the Board that heads a
// By Spec lane, on one line: a chevron, the ID, a kind badge (none for a
// Requirement), the title, the needs-you dot and the headline, then how many
// Tickets the lane shows. The line stays at the left while the columns
// scroll sideways. The chevron folds the lane; the rest opens the Spec.
import { KindBadge } from "../controls/KindBadge";
import { countStates, describeSpec } from "../inbox/describeSpec";
import { ShortId } from "../inbox/ShortId";
import type { ProjectSpec, ProjectTicket } from "../live";
import { NeedsYouDot, useNeedsMe } from "../needsme/context";
import type { RowLink } from "../ticket/TicketLine";

export interface LaneHeaderProps {
  id: string;
  // spec is missing while the Spec isn't loaded yet; the header then shows
  // its ID as its title.
  spec?: ProjectSpec;
  // all is every one of the Spec's Tickets, finished ones too.
  all: ProjectTicket[];
  // shown is how many of the Spec's Tickets the lane shows.
  shown: number;
  link: RowLink;
  collapsed: boolean;
  onToggle(): void;
}

export function LaneHeader({ id, spec, all, shown, link, collapsed, onToggle }: LaneHeaderProps) {
  const { reasonOf } = useNeedsMe();
  const title = spec?.title ?? id;
  const { kind, headline } = describeSpec(spec, all, spec?.state_summary ?? countStates(all), reasonOf);
  return (
    <section className={`lane-header${reasonOf(id) ? " needs" : ""}`} aria-label={title}>
      <div className="lane-header-line">
        <button className="chev" aria-expanded={!collapsed} aria-label={title} onClick={onToggle}>
          {collapsed ? "▸" : "▾"}
        </button>
        <a className="lane-header-link" {...link}>
          <ShortId id={id} />
          {kind !== "Requirement" && <KindBadge kind={kind} />}
          <span className="lane-header-title">{title}</span>
          <NeedsYouDot id={id} />
          <span className="lane-header-headline">{headline}</span>
          <span className="lane-header-count">{shown} shown</span>
        </a>
      </div>
    </section>
  );
}
