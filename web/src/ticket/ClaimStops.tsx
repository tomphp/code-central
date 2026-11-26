import type { ClaimView } from "../api/types";
import { timeOfDay } from "../format";

// ClaimStops is each Stop of a Claim in its row: when it was stopped, and
// when it was resumed, once it has been.
export function ClaimStops({ stops }: { stops: ClaimView["stops"] }) {
  return stops?.map((stop) => (
    <span key={stop.stopped}>
      {" · "}
      <span className="claim-field">Stopped {timeOfDay(stop.stopped)}</span>
      {stop.resumed && (
        <>
          {" · "}
          <span className="claim-field">Resumed {timeOfDay(stop.resumed)}</span>
        </>
      )}
    </span>
  ));
}
