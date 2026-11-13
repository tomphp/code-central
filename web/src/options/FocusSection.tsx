import { useEffect, useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import {
  NEEDS_ME_REASONS,
  type ApiError,
  type NeedsMeReason,
  type ProjectRow,
} from "../api/types";
import { Button } from "../controls/Button";
import { REASON_LABEL } from "../format";
import { Refusal } from "./Refusal";

// FocusSection is the Project's Focus Kinds (Spec code-central-550): a
// "Show in Focus" checkbox per Needs Me reason, in Needs Me group order,
// checked unless the reason is in the Project's off list. Save sends the
// whole off list. Like the other sections it follows the Project as read.
export function FocusSection({ api, project }: { api: Api; project: ProjectRow }) {
  const stored = JSON.stringify(project.focus_kinds.off);
  const [off, setOff] = useState<NeedsMeReason[]>(project.focus_kinds.off);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  useEffect(() => setOff(JSON.parse(stored) as NeedsMeReason[]), [stored]);

  const toggle = (reason: NeedsMeReason, shown: boolean) => {
    setOff((o) => NEEDS_ME_REASONS.filter((r) => (r === reason ? !shown : o.includes(r))));
    setError(undefined);
    setSaved(false);
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    api.setFocusKinds(project.key, off).then(
      (v) => {
        setOff(v.focus_kinds.off);
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const changed = JSON.stringify(off) !== stored;

  return (
    <fieldset id="project-options-focus" className="project-options-section" disabled={project.archived}>
      <legend>Focus</legend>
      <p className="project-options-hint">
        A reason that is off stays in Needs Me, its count, its dots and
        Notifications, but its items don&apos;t show in Focus Mode or open the
        Focus popup.
      </p>
      <form onSubmit={save}>
        <table className="project-options-focus">
          <thead>
            <tr>
              <th scope="col">Needs Me reason</th>
              <th scope="col">Show in Focus</th>
            </tr>
          </thead>
          <tbody>
            {NEEDS_ME_REASONS.map((reason) => (
              <tr key={reason}>
                <th scope="row">{REASON_LABEL[reason]}</th>
                <td>
                  <input
                    type="checkbox"
                    aria-label={REASON_LABEL[reason]}
                    checked={!off.includes(reason)}
                    onChange={(e) => toggle(reason, e.target.checked)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {error && <Refusal id={`${id}-error`} error={error} />}
        <div className="project-options-actions">
          <Button type="submit" variant="primary" disabled={!changed}>
            Save
          </Button>
          {saved && !changed && (
            <span role="status" className="muted">
              Saved
            </span>
          )}
        </div>
      </form>
    </fieldset>
  );
}
