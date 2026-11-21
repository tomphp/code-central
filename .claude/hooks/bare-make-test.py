#!/usr/bin/env python3
"""PreToolUse hook: refuse a Bash command that pipes `make test` into another command,
or that runs all of vitest or the Go suite outside `make test`.

A pipe reports the last command's status, not the suite's, and `make test`
already trims its output and prints the log path on failure.
"""
import json
import shlex
import sys

SUITE_TARGETS = {"test", "test-verbose"}
LIST_OPERATORS = {";", "&&", "||", "&"}
PIPE_OPERATORS = {"|", "|&"}
WRAPPERS = {"time", "nice", "env"}


def is_make_test(words):
    if not words or words[0] != "make":
        return False
    return any(w in SUITE_TARGETS for w in words[1:] if not w.startswith("-"))


def is_full_suite(words):
    while words and ("=" in words[0] or words[0] in WRAPPERS):
        words = words[1:]
    if words[:2] == ["go", "test"]:
        return "./..." in words
    if words[:1] == ["make"]:
        targets = [w for w in words[1:] if not w.startswith("-") and "=" not in w]
        has_filter = any(w.startswith("F=") and len(w) > 2 for w in words[1:])
        return "vitest" in targets and not has_filter
    return False


def pipeline_stages(command):
    """Yield each line's stages: lists of words, None marking a break between pipelines."""
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
        yield stages


def full_suite_command(command):
    return any(
        words is not None and is_full_suite(words)
        for stages in pipeline_stages(command)
        for words in stages
    )


def piped_make_test(command):
    for stages in pipeline_stages(command):
        # A stage is piped if another follows it in the same pipeline.
        for i, words in enumerate(stages[:-1]):
            if words is not None and stages[i + 1] is not None and is_make_test(words):
                return True
    return False


def main():
    event = json.load(sys.stdin)
    if event.get("tool_name") != "Bash":
        return
    command = (event.get("tool_input") or {}).get("command", "")
    if full_suite_command(command):
        json.dump({"hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": (
                "For the whole suite, run `make test` bare; for part of it, "
                "`make vitest F=<file>`, `make vitest-related F=<source>` or `go test ./<pkg>`."
            ),
        }}, sys.stdout)
    elif piped_make_test(command):
        json.dump({"hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": (
                "Run `make test` (or `make test-verbose`) bare, not piped into another "
                "command: a pipe reports the pipe's status, not the suite's. Its output is "
                "already trimmed; on a failure it prints the path of the full log, so read "
                "that file."
            ),
        }}, sys.stdout)


if __name__ == "__main__":
    main()
