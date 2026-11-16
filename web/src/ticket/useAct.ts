// useAct runs one human action at a time and keeps its error to show.
// On success it does nothing more: the service's change notice makes the
// whole UI refetch, by the invalidation rule in live.tsx.
import { useState } from "react";

export function useAct() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  // run says whether the action succeeded.
  async function run(action: () => Promise<unknown>): Promise<boolean> {
    setBusy(true);
    setError(undefined);
    try {
      await action();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setBusy(false);
    }
  }
  return { busy, error, run };
}
