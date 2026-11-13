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
import shlex
import sys

SOURCE = re.compile(r"[\w./-]+\.(?:go|ts|tsx|js|mjs|md|ya?ml|json|toml|css)\b")
PY_EDIT = re.compile(r"\.replace\(|re\.sub\(")
PY_WRITE = re.compile(r"""open\([^)]*['"]w['"]|\.write_text\(""")
PY_MANY = re.compile(r"\bfor\b.*\bin\b|glob\(|os\.walk\(")
SED_INPLACE =re.compile(r"(?:^|[;&|\n]\s*)sed\s+(?:-\w+\s+)*-i\b")
HEREDOC = re.compile(r"<<-?[ \t]*(['\"]?)(\w+)\1[^\n]*\n(.*?)^[ \t]*\2[ \t]*$", re.S | re.M)
OPEN_CALL = re.compile(r"""\bopen\(\s*([^,()]+?)\s*(?:,\s*([^)]*))?\)""")
TEXT_CALL = re.compile(r"(\w+(?:\([^()]*\))?)\.(read|write)_text\(")
QUOTED = re.compile(r"""'[^']*'|"[^"]*\"""")


def split_heredocs(command):
    """Returns the shell text, with each heredoc body cut out, and the bodies."""
    bodies = [m.group(3) for m in HEREDOC.finditer(command)]
    return HEREDOC.sub("", command), bodies


def rewrites_one_path(body):
    """True when the Python opens one path for reading and the same one for writing."""
    reads, writes = set(), set()
    for path, mode in OPEN_CALL.findall(body):
        (writes if "w" in mode else reads).add(path)
    for path, kind in TEXT_CALL.findall(body):
        (writes if kind == "write" else reads).add(path)
    return bool(reads & writes)


def sed_operands(shell):
    """The file operands of the `sed -i` in the shell text: the words after its expression."""
    for segment in re.split(r"[;&|\n]", shell):
        try:
            words = shlex.split(segment)
        except ValueError:
            continue
        if not words or words[0] != "sed" or not any(re.fullmatch(r"-\w*i\w*|--in-place.*", w) for w in words[1:]):
            continue
        rest = [w for w in words[1:] if not w.startswith("-")]
        scripted = any(w in ("-e", "-f") or w.startswith("--expression") for w in words[1:])
        return rest if scripted else rest[1:]
    return []


def edits_one_file(command):
    shell, bodies = split_heredocs(command)
    if any("python" in shell and rewrites_one_path(b) for b in bodies) or (
            not bodies and re.search(r"\bpython3?\b", command) and rewrites_one_path(command)):
        return True
    unquoted = QUOTED.sub("''", shell)
    if re.search(r"\bxargs\b|\bfind\b|\bfor\b", unquoted):
        return False
    if SED_INPLACE.search(shell):
        operands = sed_operands(shell)
        return len(operands) <= 1 and not any("*" in w for w in operands)
    if len(set(SOURCE.findall(shell))) > 1 or "*" in unquoted:
        return False
    # Without a same-path rewrite, a body that loops or globs is a many-file rename.
    return bool(re.search(r"\bpython3?\b", shell) and any(
        PY_EDIT.search(b) and PY_WRITE.search(b) and not PY_MANY.search(b) for b in bodies or [command]))


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
