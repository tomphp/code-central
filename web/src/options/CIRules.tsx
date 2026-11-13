import { useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import type { ApiError, CIRule, CIRuleChange, ProjectRow } from "../api/types";
import { Field } from "../controls/Field";
import { Select } from "../controls/Select";

// ruleWatches is what a CI Rule watches, as its Spec's title says it.
const ruleWatches = (r: CIRuleChange) =>
  `${r.workflow === "any" ? "any workflow" : r.workflow} on ${r.branch}`;

const NO_RULE: CIRuleChange = { workflow: "", branch: "", state: "Draft" };

// CIRules, in the CI section, lists the Project's CI Rules ("when workflow
// X fails on branch Y, open a Ticket to fix it") and adds, edits and
// removes them. Each rule has its own Spec, which removing the rule keeps.
export function CIRules({ api, project }: { api: Api; project: ProjectRow }) {
  const [editing, setEditing] = useState<string>();
  const id = useId();
  const { rules } = project.ci;

  return (
    <fieldset className="project-options-rules-section">
      <legend>CI Rules</legend>
      <p className="project-options-hint">
        A CI Rule says &ldquo;when this workflow fails on this branch, open a
        Ticket to fix it&rdquo;, on the rule&apos;s own Spec. For now rules are
        only recorded: none opens a Ticket yet.
      </p>
      {rules.length === 0 ? (
        <p className="muted">No CI Rules yet.</p>
      ) : (
        <ul className="project-options-rules" aria-label="CI Rules">
          {rules.map((r) =>
            editing === r.spec ? (
              <li key={r.spec}>
                <RuleForm
                  label={`Edit ${ruleWatches(r)}`}
                  initial={r}
                  submit="Save"
                  save={(change) =>
                    api
                      .editCIRule(project.key, r.spec, change)
                      .then(() => setEditing(undefined))
                  }
                  cancel={() => setEditing(undefined)}
                />
              </li>
            ) : (
              <RuleRow
                key={r.spec}
                rule={r}
                removeHint={`${id}-remove`}
                edit={() => setEditing(r.spec)}
                remove={() => api.removeCIRule(project.key, r.spec)}
              />
            ),
          )}
        </ul>
      )}
      <p hidden id={`${id}-remove`}>
        Removing a rule keeps its Spec, as the record of its Tickets.
      </p>
      <RuleForm
        label="Add a CI Rule"
        initial={NO_RULE}
        submit="Add rule"
        save={(change) => api.addCIRule(project.key, change)}
        clearOnSave
      />
    </fieldset>
  );
}

// RuleRow is one CI Rule, with its Edit and Remove buttons.
function RuleRow({
  rule,
  removeHint,
  edit,
  remove,
}: {
  rule: CIRule;
  removeHint: string;
  edit(): void;
  remove(): Promise<unknown>;
}) {
  const [error, setError] = useState<ApiError>();
  const watches = ruleWatches(rule);
  return (
    <li>
      <span>
        <strong>{watches}</strong>: opens {rule.state} Tickets on {rule.spec}
      </span>{" "}
      <button
        type="button"
        className="btn"
        aria-label={`Edit ${watches}`}
        onClick={edit}
      >
        Edit
      </button>{" "}
      <button
        type="button"
        className="btn"
        aria-label={`Remove ${watches}`}
        aria-describedby={removeHint}
        onClick={() => remove().catch(setError)}
      >
        Remove
      </button>
      {error && (
        <p className="warning" role="alert">
          {error.message}
        </p>
      )}
    </li>
  );
}

// RuleForm is a CI Rule's fields: adding one, or editing one in place.
function RuleForm(props: {
  label: string;
  initial: CIRuleChange;
  submit: string;
  save(change: CIRuleChange): Promise<unknown>;
  cancel?(): void;
  clearOnSave?: boolean;
}) {
  const { label, initial, submit, save, cancel, clearOnSave } = props;
  const [draft, setDraft] = useState<CIRuleChange>({
    workflow: initial.workflow,
    branch: initial.branch,
    state: initial.state,
  });
  const [error, setError] = useState<ApiError>();
  const id = useId();

  const edit = (patch: Partial<CIRuleChange>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
  };
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    save(draft).then(
      () => clearOnSave && setDraft(NO_RULE),
      (e: ApiError) => setError(e),
    );
  };
  const invalid = (field: string) =>
    error?.field === field
      ? { "aria-invalid": true, "aria-describedby": `${id}-error` }
      : undefined;
  const filled = draft.workflow.trim() !== "" && draft.branch.trim() !== "";

  return (
    <form
      aria-label={label}
      className="project-options-rule"
      onSubmit={onSubmit}
    >
      <Field
        label="Workflow"
        hintId={`${id}-workflow`}
        hint={
          <>
        A workflow file name, such as ci.yml, or any for every workflow but the
        Ignored ones.
          </>
        }
      >
        <input
          value={draft.workflow}
          placeholder="ci.yml or any"
          onChange={(e) => edit({ workflow: e.target.value })}
          aria-describedby={`${id}-workflow`}
          {...invalid("workflow")}
        />
      </Field>
      <Field
        label="Branch"
        hintId={`${id}-branch`}
        hint={
          <>
        An exact branch name, with no globs.
          </>
        }
      >
        <input
          value={draft.branch}
          placeholder="main"
          onChange={(e) => edit({ branch: e.target.value })}
          aria-describedby={`${id}-branch`}
          {...invalid("branch")}
        />
      </Field>
      <label className="project-options-field">
        Tickets open as{" "}
        <Select
          value={draft.state}
          onChange={(v) => edit({ state: v as CIRuleChange["state"] })}
          options={[{ value: "Draft" }, { value: "Ready" }]}
          {...invalid("state")}
        />
      </label>
      {error && (
        <p className="warning" role="alert" id={`${id}-error`}>
          {error.message}
        </p>
      )}
      <div className="project-options-actions">
        <button type="submit" className="btn primary" disabled={!filled}>
          {submit}
        </button>
        {cancel && (
          <button type="button" className="btn" onClick={cancel}>
            Cancel
          </button>
        )}
      </div>
    </form>
  );
}
