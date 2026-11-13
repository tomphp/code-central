import type { ApiError } from "../api/types";

// Refusal is a save the service refused: its message, then its fix when
// it gives one, as a refused CI Tracking or GitHub Issues turn-on does
// (code-central-252, code-central-284).
export function Refusal({ id, error }: { id: string; error: ApiError }) {
  return (
    <p className="warning" role="alert" id={id}>
      {error.message}
      {error.fix && (
        <>
          <br />
          {error.fix}
        </>
      )}
    </p>
  );
}
