// SpecPopup is the Board's popup for a Spec: the Requirement, Map or Spec
// pane, whichever fits as App chooses it for the Inbox, unchanged and with
// its actions, in the wide modal shell. It reads the Spec on the live
// version, so it follows the feed, and shows Loading… or the read's error
// as the Inbox's detail pane does. Choosing a Ticket, or a link to another
// Spec, switches the popup. No Ticket is shown as selected in it.
// SpecPopupView is the popup over a read handed in.
import { projectOf } from "../api/api";
import type { ApiError, ProjectRow, SpecDetail } from "../api/types";
import { useSpec } from "../live";
import { MapPane } from "../map/MapPane";
import { RequirementPane } from "../requirement/RequirementPane";
import { SpecPane } from "../spec/SpecPane";
import { SpecPaneNote } from "../spec/SpecPaneNote";
import type { Link } from "../url";
import { PopupShell } from "../controls/PopupShell";

export interface SpecPopupProps {
  now: Date;
  id: string;
  projects: ProjectRow[];
  onClose(): void;
  // open switches the popup to another Spec's or a Ticket's.
  open(id: string): void;
  // specLink links another Spec's popup.
  specLink(id: string): Link;
}

export function SpecPopup(props: SpecPopupProps) {
  const spec = useSpec(projectOf(props.id), props.id);
  return <SpecPopupView {...props} spec={spec} />;
}

export interface SpecPopupViewProps extends SpecPopupProps {
  // spec is the read of the Spec: none yet while loading.
  spec: { data?: SpecDetail; error?: ApiError };
}

export function SpecPopupView({ now, id, projects, spec, onClose, open, specLink }: SpecPopupViewProps) {
  const project = projectOf(id);
  const archived = !!projects.find((p) => p.key === project)?.archived;
  const data = spec.data?.id === id ? spec.data : undefined;
  return (
    <PopupShell label="Spec popup" size="wide" onClose={onClose}>
      {data?.requirement ? (
        <RequirementPane
          spec={{ ...data, requirement: data.requirement }}
          archived={archived}
          now={now}
          select={open}
          specLink={specLink}
        />
      ) : data?.map ? (
        <MapPane spec={{ ...data, map: data.map }} archived={archived} now={now} select={open} specLink={specLink} written={open} />
      ) : data ? (
        <SpecPane spec={data} archived={archived} now={now} select={open} />
      ) : (
        <SpecPaneNote error={spec.error?.message} />
      )}
    </PopupShell>
  );
}
