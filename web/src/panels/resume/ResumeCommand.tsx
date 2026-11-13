// ResumeCommand shows a copyable command to get back into a session, or
// says that no session ID is recorded. Shared by any panel that offers to
// bring a session back (the Failed panel's report has one too).
import { useState } from "react";
import { Button } from "../../controls/Button";

// Session is what a Claim (or a Failure Report's copy of it) records about
// getting back into its session. Code Central never parses any of it.
export interface Session {
  sessionId?: string;
  workingDir?: string;
  resumeCommand?: string;
}

// composeResume is `cd <dir> && <command>`, leaving out the cd when there
// is no directory. Without a Resume Command it falls back to
// `claude --resume <id>`; with neither, there is nothing to run.
export function composeResume({ sessionId, workingDir, resumeCommand }: Session): string | undefined {
  const command = resumeCommand || (sessionId ? `claude --resume ${sessionId}` : undefined);
  if (!command) return undefined;
  return workingDir ? `cd ${shellQuote(workingDir)} && ${command}` : command;
}

// shellQuote leaves a plain path as it is and single-quotes anything else,
// so the cd survives spaces and the like.
function shellQuote(s: string): string {
  return /^[\w@%+=:,./~-]+$/.test(s) ? s : `'${s.replace(/'/g, `'\\''`)}'`;
}

export function ResumeCommand(session: Session) {
  const [copied, setCopied] = useState(false);
  const command = composeResume(session);
  if (!command) {
    return <p className="muted small">No session ID is recorded, so there is no session to resume.</p>;
  }
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
      <Button type="button"  onClick={copy}>
        {copied ? "Copied" : "Copy"}
      </Button>
    </p>
  );
}
