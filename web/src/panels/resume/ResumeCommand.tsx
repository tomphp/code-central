// ResumeCommand shows a copyable `claude --resume <id>` for a session, or
// says that no session ID is recorded. Shared by any panel that offers to
// bring a session back (the Failed panel's report has one too).
import { useState } from "react";

export function ResumeCommand({ sessionId }: { sessionId?: string }) {
  const [copied, setCopied] = useState(false);
  if (!sessionId) {
    return <p className="muted small">No session ID is recorded, so there is no session to resume.</p>;
  }
  const command = `claude --resume ${sessionId}`;
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
    } catch {
      // No clipboard (an insecure origin): the command is still there to select.
    }
  };
  return (
    <p className="resume-command">
      <code>{command}</code>{" "}
      <button type="button" className="btn" onClick={copy}>
        {copied ? "Copied" : "Copy"}
      </button>
    </p>
  );
}
