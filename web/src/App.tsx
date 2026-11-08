import type { Api } from "./api/api";
import { projectOf } from "./api/api";
import { Inbox } from "./inbox/Inbox";
import { useInboxData, useLiveVersion, useSpec, useTicket } from "./live";
import { defaultTicket, needsMeCount, progress, reasonsOf } from "./requirement/progress";
import { useNeedsMeTitle } from "./title";
import { useUiLocation } from "./url";

export interface AppProps {
  api: Api;
  // now is the clock "how long it waited" is measured against; tests pin it.
  now?: () => Date;
}

export function App({ api, now = () => new Date() }: AppProps) {
  const [loc, navigate] = useUiLocation();
  const version = useLiveVersion(api);
  const inbox = useInboxData(api, version);
  const spec = useSpec(api, loc.requirement && projectOf(loc.requirement), loc.requirement, version);
  const requirement = spec.data?.id === loc.requirement && spec.data?.requirement ? { ...spec.data, requirement: spec.data.requirement } : undefined;
  const reasons = reasonsOf(inbox.data?.needsMe);
  // Within a Requirement with no Ticket chosen, the pane opens on its default one.
  const selected =
    loc.ticket ?? (requirement ? defaultTicket(progress(requirement.requirement, requirement.tickets, reasons)) : undefined);
  const ticket = useTicket(api, selected && projectOf(selected), selected, version);
  useNeedsMeTitle(inbox.data && needsMeCount(inbox.data.needsMe));

  // One layout in v1. A later view (focus queue, board) is another case
  // here, reusing the same data and the shared Ticket pane and panels.
  switch (loc.view) {
    case "inbox":
      return (
        <>
          {inbox.error && <p className="banner">{inbox.error.message}</p>}
          <Inbox
            api={api}
            now={now()}
            data={inbox.data}
            loc={{ ...loc, ticket: selected }}
            navigate={navigate}
            requirement={requirement}
            requirementError={spec.error && spec.data?.id !== loc.requirement ? spec.error.message : undefined}
            ticket={ticket.data}
            ticketError={ticket.error && ticket.data?.id !== selected ? ticket.error.message : undefined}
          />
        </>
      );
  }
}
