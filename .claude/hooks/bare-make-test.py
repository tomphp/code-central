#!/usr/bin/env python3
"""PreToolUse hook: refuse a Bash command that pipes `make test` or `make e2e` into another command.

A pipe reports the last command's status, not the suite's, and both
already trim their output and print the log path on failure.
"""
import json
import shlex
import sys

SUITE_TARGETS = {"test", "test-verbose", "e2e"}
LIST_OPERATORS = {";", "&&", "||", "&"}
PIPE_OPERATORS = {"|", "|&"}


def is_make_test(words):
    if not words or words[0] != "make":
        return False
    return any(w in SUITE_TARGETS for w in words[1:] if not w.startswith("-"))


def piped_make_test(command):
    for line in command.split("\n"):
        lexer = shlex.shlex(line, posix=True, punctuation_chars=True)
        lexer.whitespace_split = True
        try:
            tokens = list(lexer)
        except ValueError:
            continue
        stages = [[]]
        for token in tokens:
            if token in LIST_OPERATORS:
                stages.append(None)
                stages.append([])
            elif token in PIPE_OPERATORS:
                stages.append([])
            elif stages[-1] is not None:
                stages[-1].append(token)
        # None marks a break between pipelines; a stage is piped if another follows it in the same pipeline.
        for i, words in enumerate(stages[:-1]):
            if words is not None and stages[i + 1] is not None and is_make_test(words):
                return True
    return False


def main():
    event = json.load(sys.stdin)
    if event.get("tool_name") != "Bash":
        return
    if piped_make_test((event.get("tool_input") or {}).get("command", "")):
        json.dump({"hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": (
                "Run `make test` (or `make test-verbose`, or `make e2e`) bare, not piped into "
                "another command: a pipe reports the pipe's status, not the suite's. Its "
                "output is already trimmed; on a failure it prints the path of the full log, "
                "so read that file."
            ),
        }}, sys.stdout)


if __name__ == "__main__":
    main()
