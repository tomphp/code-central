#!/usr/bin/env python3
"""PreToolUse hook: refuse a Bash command that edits one file in place.

`sed -i` and a `python3` heredoc that reads a file, `.replace()`s in it
and writes it back are what agents reach for instead of the Edit tool.
They hide the change from the harness, differ between BSD and GNU sed,
and fail silently when the text isn't there. Edit does the same job, and
it refuses a replacement that matches nothing. A command that rewrites
several files, as a rename does, is let through.
"""
import json
import re
import sys

SOURCE = re.compile(r"[\w./-]+\.(?:go|ts|tsx|js|mjs|md|ya?ml|json|toml|css)\b")
PY_EDIT = re.compile(r"\.replace\(|re\.sub\(")
PY_WRITE = re.compile(r"""open\([^)]*['"]w['"]|\.write_text\(""")
SED_INPLACE = re.compile(r"(?:^|[;&|\n]\s*)sed\s+(?:-\w+\s+)*-i\b")


def edits_one_file(command):
    files = set(SOURCE.findall(command))
    if len(files) > 1 or re.search(r"\bxargs\b|\bfind\b|\bfor\b|\*", command):
        return False
    if SED_INPLACE.search(command):
        return True
    return bool(re.search(r"\bpython3?\b", command) and PY_EDIT.search(command) and PY_WRITE.search(command))


def main():
    event = json.load(sys.stdin)
    if event.get("tool_name") != "Bash":
        return
    if edits_one_file((event.get("tool_input") or {}).get("command", "")):
        json.dump({"hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": (
                "Edit the file with the Edit tool, not `sed -i` or a python replace. "
                "Read the lines you are changing first (Read with offset and limit, "
                "which Edit accepts), then Edit. Edit fails when old_string isn't "
                "found or isn't unique, so a missed change can't pass silently."
            ),
        }}, sys.stdout)


if __name__ == "__main__":
    main()
