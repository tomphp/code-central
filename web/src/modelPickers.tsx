// What the model and effort pickers offer (Spec code-central-301), in
// Project Options' Models section and on the Ticket panel alike.
import type { ReportedModels } from "./api/types";

// modelSuggestions are what a model picker suggests: the Reported
// Models' aliases, then their model IDs, each once.
export const modelSuggestions = (reported: ReportedModels) => [...new Set([...reported.aliases, ...reported.model_ids])];

// effortOptions are what an effort picker offers: the valid levels, plus
// the current value if a later report dropped it, so it stays shown.
export const effortOptions = (valid: string[], current: string) => (current && !valid.includes(current) ? [...valid, current] : valid);

// ModelSuggestions is the list a model picker names in its list
// attribute; with nothing to suggest there is none.
export function ModelSuggestions({ id, suggestions }: { id: string; suggestions: string[] }) {
  if (suggestions.length === 0) return null;
  return (
    <datalist id={id}>
      {suggestions.map((m) => (
        <option key={m} value={m} />
      ))}
    </datalist>
  );
}
