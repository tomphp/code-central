// useAct runs one human action at a time and keeps its error to show.
// On success it does nothing more: the service's change notice makes the
// whole UI refetch.
import { useState } from "react";

export function useAct() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    setError(undefined);
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  return { busy, error, run };
}
