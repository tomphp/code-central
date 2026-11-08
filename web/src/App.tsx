import type { Api } from "./api/api";
import { projectOf } from "./api/api";
import { Focus } from "./focus/Focus";
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

  // Each view is a layout over the same live data; a later one (a board)
  // is another case here, reusing the shared Ticket pane and panels.
  const banner = inbox.error && <p className="banner">{inbox.error.message}</p>;
  switch (loc.view) {
    case "focus":
      return (
        <>
          {banner}
          <Focus data={inbox.data} loc={loc} navigate={navigate} />
        </>
      );
    case "inbox":
      return (
        <>
          {banner}
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
