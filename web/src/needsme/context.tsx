// The Needs Me read model as App provides it to every view: built once per
// Inbox load. Outside a provider it is the model of no data, which needs
// nothing.
import { createContext, useContext, type ReactNode } from "react";
import { needsMeOf, type NeedsMe } from "./model";

const NeedsMeContext = createContext<NeedsMe>(needsMeOf(undefined));

export function NeedsMeProvider({ value, children }: { value: NeedsMe; children: ReactNode }) {
  return <NeedsMeContext.Provider value={value}>{children}</NeedsMeContext.Provider>;
}

// useNeedsMe is the Needs Me read model of the last Inbox load.
export function useNeedsMe(): NeedsMe {
  return useContext(NeedsMeContext);
}

// NeedsYouDot is the needs-you dot of a Ticket or Spec, in its reason's
// colour, drawn while the read model says it needs the human.
export function NeedsYouDot({ id }: { id: string }) {
  const reason = useNeedsMe().reasonOf(id);
  return reason ? <span className={`dot reason-${reason}`} role="img" aria-label="Needs you" /> : null;
}
