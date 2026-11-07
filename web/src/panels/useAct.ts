// useAct runs a panel's human actions one at a time: busy while one is in
// flight, and the refusal's message if it fails. On success it does
// nothing more: the service's notice refetches the UI, which re-keys the
// panel.
import { useState } from "react";

export function useAct() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const run = async (action: () => Promise<unknown>) => {
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  return { busy, error, run };
}
