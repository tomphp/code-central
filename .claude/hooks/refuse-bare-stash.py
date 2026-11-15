#!/usr/bin/env python3
"""PreToolUse hook: refuse a Bash command that uses the shared git stash unsafely.

Every worktree and the main checkout share one stash stack. A bare
`git stash` or `git stash push` without `-m` can't be told from another
session's entry, and `git stash pop` applies and drops whatever is on top,
which may be another session's work.
"""
import json
import re
import shlex
import sys

SEPARATORS = re.compile(r"&&|\|\||[;&|\n]")
OPTIONS_WITH_VALUE = {"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--exec-path"}


def has_message(args):
    for w in args:
        if w == "--":
            return False
        if w == "--message" or w.startswith("--message="):
            return True
        if re.fullmatch(r"-[a-zA-Z]*m[^-]*", w):
            return True
    return False


def stash_args(words):
    """The words after `git stash`, or None when the words are not a git stash."""
    if not words or words[0] != "git":
        return None
    i = 1
    while i < len(words) and words[i].startswith("-"):
        i += 2 if words[i] in OPTIONS_WITH_VALUE else 1
    if i < len(words) and words[i] == "stash":
        return words[i + 1:]
    return None


def unsafe_stash(command):
    for segment in SEPARATORS.split(command):
        try:
            args = stash_args(shlex.split(segment))
        except ValueError:
            continue
        if args is None:
            continue
        sub = next((w for w in args if not w.startswith("-")), None)
        if sub is None or sub == "pop":
            return True
        if sub in ("push", "save") and not has_message(args):
            return True
    return False


def main():
    event = json.load(sys.stdin)
    if event.get("tool_name") != "Bash":
        return
    if unsafe_stash((event.get("tool_input") or {}).get("command", "")):
        json.dump({"hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": (
                "Every worktree shares one git stash stack, so a bare `git stash`, "
                "`git stash push` without -m, or `git stash pop` can apply or drop "
                "another session's changes. Set work aside with a temporary WIP "
                "commit instead, or run the base's tests in a `git worktree add` "
                "of the base."
            ),
        }}, sys.stdout)


if __name__ == "__main__":
    main()
