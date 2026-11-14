// SpecPopup is the Board's popup for a Spec: the Requirement, Map or Spec
// pane, whichever fits as App chooses it for the Inbox, unchanged and with
// its actions, in the wide modal shell. It reads the Spec on the live
// version, so it follows the feed, and shows Loading… or the read's error
// as the Inbox's detail pane does. Choosing a Ticket, or a link to another
// Spec, switches the popup. No Ticket is shown as selected in it.
import type { Api } from "../api/api";
import { projectOf } from "../api/api";
import type { ProjectRow } from "../api/types";
import { useSpec } from "../live";
import { MapPane } from "../map/MapPane";
import { RequirementPane } from "../requirement/RequirementPane";
import { SpecPane } from "../spec/SpecPane";
import type { Link } from "../url";
import { PopupShell } from "./PopupShell";

export interface SpecPopupProps {
  api: Api;
  now: Date;
  version: number;
  id: string;
  projects: ProjectRow[];
  onClose(): void;
  // open switches the popup to another Spec's or a Ticket's.
  open(id: string): void;
  // specLink links another Spec's popup.
  specLink(id: string): Link;
}

export function SpecPopup({ api, now, version, id, projects, onClose, open, specLink }: SpecPopupProps) {
  const project = projectOf(id);
  const spec = useSpec(api, project, id, version);
  const archived = !!projects.find((p) => p.key === project)?.archived;
  const data = spec.data?.id === id ? spec.data : undefined;
  return (
    <PopupShell label="Spec popup" onClose={onClose}>
      {data?.requirement ? (
        <RequirementPane
          api={api}
          spec={{ ...data, requirement: data.requirement }}
          archived={archived}
          now={now}
          select={open}
          specLink={specLink}
        />
      ) : data?.map ? (
        <MapPane api={api} spec={{ ...data, map: data.map }} now={now} select={open} specLink={specLink} written={open} />
      ) : data ? (
        <SpecPane api={api} spec={data} archived={archived} now={now} select={open} />
      ) : (
        <p className="empty">{spec.error ? spec.error.message : "Loading…"}</p>
      )}
    </PopupShell>
  );
}
