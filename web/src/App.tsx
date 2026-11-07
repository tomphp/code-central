import type { Api } from "./api/api";
import { projectOf } from "./api/api";
import { Inbox } from "./inbox/Inbox";
import { useInboxData, useLiveVersion, useTicket } from "./live";
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
  const ticket = useTicket(api, loc.ticket && projectOf(loc.ticket), loc.ticket, version);
  useNeedsMeTitle(inbox.data?.needsMe.count);

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
            loc={loc}
            navigate={navigate}
            ticket={ticket.data}
            ticketError={ticket.error && ticket.data?.id !== loc.ticket ?ticket.error.message : undefined}
          />
        </>
      );
  }
}
