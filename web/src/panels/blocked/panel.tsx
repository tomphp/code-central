import type { PanelDef } from "../ActionPanel";
import { Blocked } from "./Blocked";

export const panel: PanelDef = { reasons: ["blocked_by_cancelled"], Component: Blocked };
