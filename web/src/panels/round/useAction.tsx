// useAction, for the Round panel and its shared answer form: one request
// at a time, keeping its refusal to show.
import { useState } from "react";

export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const run = async (f: () => Promise<unknown>): Promise<boolean> => {
    setBusy(true);
    try {
      await f();
      setError(undefined);
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setBusy(false);
    }
  };
  const alert = error && (
    <p role="alert" className="warning">
      {error}
    </p>
  );
  return { busy, alert, run };
}
