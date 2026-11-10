#!/usr/bin/env python3
"""PreToolUse hook: refuse whole-file reads of long source files.

A whole-file `cat` or an unbounded Read of a long file puts all of it in
context, where every later call re-reads it. This refuses them and points
at a narrow read instead. A `cat` piped into another command or
redirected is let through, since none of it reaches context whole.
"""
import json
import os
import re
import shlex
import sys

LIMIT = 300
SOURCE = re.compile(r"\.(go|ts|tsx|js|mjs)$")
SAVED_OUTPUT = re.compile(r"/tool-results/[^/]+\.txt$")


def lines(path, cwd):
    p = path if os.path.isabs(path) else os.path.join(cwd, path)
    try:
        with open(p, "rb") as f:
            return sum(1 for _ in f)
    except OSError:
        return 0


def watched(path):
    return SOURCE.search(path) or SAVED_OUTPUT.search(path)


def long_cats(command, cwd):
    """The long watched files a command cats whole, outside a pipe."""
    out = []
    for segment in re.split(r"&&|\|\||;|\n", command):
        piped = segment.split("|")
        try:
            words = shlex.split(piped[0])
        except ValueError:
            continue
        if not words or words[0] != "cat" or len(piped) > 1 or ">" in piped[0]:
            continue
        for w in words[1:]:
            if not w.startswith("-") and watched(w):
                n = lines(w, cwd)
                if n > LIMIT:
                    out.append((w, n))
    return out


def deny(found):
    files = ", ".join(f"{p} ({n} lines)" for p, n in found)
    reason = (
        f"Read refused: {files} is too long to read whole. Find what you need "
        "with `grep -n` (or `go doc ./pkg Name` for a Go API), then read just "
        "that range with `sed -n A,Bp` or Read with offset and limit."
    )
    json.dump({"hookSpecificOutput": {
        "hookEventName": "PreToolUse",
        "permissionDecision": "deny",
        "permissionDecisionReason": reason,
    }}, sys.stdout)


def main():
    event = json.load(sys.stdin)
    cwd = event.get("cwd") or os.getcwd()
    tool, args = event.get("tool_name"), event.get("tool_input") or {}
    found = []
    if tool == "Bash":
        found = long_cats(args.get("command", ""), cwd)
    elif tool == "Read" and not args.get("limit"):
        path = args.get("file_path", "")
        if watched(path) and lines(path, cwd) > LIMIT:
            found = [(path, lines(path, cwd))]
    if found:
        deny(found)


if __name__ == "__main__":
    main()
