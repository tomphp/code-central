# Harnesses: Codex CLI, Copilot CLI, Gemini CLI and OpenCode

Ticket code-central-1252, for Spec code-central-1241. Read on 2026-11-26.

## Question

Could the Factory run a Claim's Turns with a coding agent other than
Claude Code? For each of Codex CLI, Copilot CLI (`copilot`), Gemini CLI and
OpenCode, this answers the Spec's eleven questions, gives a verdict on the
required core (headless run, resume, a session ID tied to the Claim, MCP
access to Code Central), sorts every Claude coupling in the Factory and in
Code Central into conceptual or naming, and proposes the Harness interface
the refactor (code-central-1254 to 1258) builds.

## Method

Public docs and source only, with no subscription and no run of any tool.
Each answer cites its doc or source file. "Unconfirmed" marks an answer the
docs don't state and the source read didn't settle; it needs a run before a
Harness relies on it.

| Tool | Version read | Published | Where |
|---|---|---|---|
| Claude Code (baseline) | 2.1.311 | installed | `factory/claudeturn.go` and its neighbours |
| Codex CLI | rust-v0.172.0 | 2026-11-24 | [openai/codex](https://github.com/openai/codex) at the tag; [learn.chatgpt.com/docs](https://learn.chatgpt.com/docs) (developers.openai.com/codex redirects there) |
| Copilot CLI | v1.0.116 | 2026-11-25 | [CLI command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference), [programmatic reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-programmatic-reference), [changelog](https://raw.githubusercontent.com/github/copilot-cli/main/changelog.md) |
| Gemini CLI | v0.71.1 | 2026-11-24 | [google-gemini/gemini-cli](https://github.com/google-gemini/gemini-cli) `main` and its `docs/` |
| OpenCode | v2.4.2 | 2026-11-26 | [anomalyco/opencode](https://github.com/anomalyco/opencode) at `dev` 314842df; [opencode.ai/docs](https://opencode.ai/docs/) |

OpenCode v2 is a rewrite: v1 ended at v1.19.6 (2026-11-17), and GitHub's
`releases/latest` still points there. Everything below is v2; a Harness
pins the major version.

## Verdict on the required core

| | Headless run | Resume | Session ID tied to the Claim | MCP access to Code Central | Verdict |
|---|---|---|---|---|---|
| Claude Code | yes | yes | Factory chooses | `.mcp.json` | meets it |
| Codex CLI | yes | yes | **captured**: Codex assigns it | `.codex/config.toml`, with `--trust` | meets it, through the self-assigned path |
| Copilot CLI | yes (event schema undocumented) | yes | Factory chooses (a UUID) | `.mcp.json` or `.github/mcp.json`, with a trust env var | meets it |
| Gemini CLI | yes | yes (in source, not the headless docs) | Factory chooses | `.gemini/settings.json`, with `--skip-trust` | meets it |
| OpenCode | yes (no result event: sum the steps) | yes | Factory chooses (`ses` prefix) | `opencode.json` | meets it |

All four can carry a Claim. Codex is the only one that needs the
self-assigned session ID path the Spec leaves unbuilt.

## Capability matrix

"Unconf." is unconfirmed; "—" is absent.

| Capability | Claude Code | Codex CLI | Copilot CLI | Gemini CLI | OpenCode |
|---|---|---|---|---|---|
| Event stream | `stream-json` | `exec --json` | `-p --output-format json` | `--output-format stream-json` | `run --format json` |
| Final result event | `result` | — (`turn.completed`/`turn.failed`) | `result` | `result` | — (`step_finish` per step) |
| Factory chooses session ID | `--session-id` | — (`thread.started.thread_id`) | `--session-id` (UUID) | `--session-id` | `-s ses_…` (creates if missing) |
| Headless resume | `--resume` | `exec resume <id>` | `--resume=<id>` | `--resume <id>` | `-s <id>` |
| Appended system prompt | `--append-system-prompt` | `-c developer_instructions=` | — (instruction files) | — (`GEMINI_SYSTEM_MD` replaces) | — (`instructions` in config) |
| Persists across resume | yes | unconf. (re-pass) | files re-read | files re-read | config re-read |
| Never prompts | `--permission-prompts none` | `exec` default (approval Never) | `-p` default | headless default | `run` default |
| Denials reported | `permission_denials` list | per item, `declined` | unconf. (per tool event) | per `tool_result`, `policy_violation` | per `tool_use`, `permission.rejected` |
| Pre-allow on resume | `--allowedTools` | `.codex/rules/*.rules` | `--allow-tool` | `--policy <file>` | config `permissions` |
| Extra directories | `--add-dir` | `--add-dir` | `--add-dir` | `--include-directories` | `external_directory` permission |
| HTTP MCP config | `.mcp.json` | `.codex/config.toml` | `.mcp.json`, `.github/mcp.json` | `.gemini/settings.json` | `opencode.json` |
| Trust gate on repo config | — | `--trust` | env var (`--trust-folder` conflicts with `--session-id`) | `--skip-trust` | none found |
| MCP tools deferred | yes (ToolSearch) | unconf. (no) | yes (`deferTools`) | no | yes (Code Mode) |
| Hook rewrites tool input | PreToolUse `updatedInput` | PreToolUse `updatedInput` | preToolUse `modifiedArgs` | BeforeTool `tool_input` | plugin `execute.before` |
| Hook after every tool | PostToolUse | PostToolUse | postToolUse | AfterTool | plugin `execute.after` |
| Hook config | `.claude/settings.json` | `.codex/hooks.json` | `.github/hooks/*.json` (also reads `.claude/settings.json`) | `.gemini/settings.json` | `.opencode/plugins/*.ts` |
| Stage skill as first prompt | `/<skill>` | `$<skill>` (unconf. in `exec`) | unconf. | `/<command>` | — in `run` |
| Skill directory | plugin, `.claude/skills` | `.agents/skills` | `.github/skills`, `.agents/skills`, `.claude/skills` | `.gemini/skills`, `.agents/skills` | `.opencode/skills`, `.agents/skills`, `.claude/skills` |
| Subagents | yes | yes | yes | yes | yes |
| Subagent usage attributed | `parent_tool_use_id` | unconf. | unconf. | — | child session |
| Token usage | per message | thread total per turn | durations only (unconf.) | `result.stats` | per step, with cost |
| Cache write tokens | yes | yes | SDK only | — | yes |
| Plan-limit events | `rate_limit_event` | — | — | — | — |
| Model flag | `--model` | `-m` | `--model` | `-m` | `--model provider/model` |
| Effort | `--effort` | `-c model_reasoning_effort=` | `--effort` | — (settings only) | `#variant` on the model |
| Model discovery | `--help` | `codex debug models` | — | `--list-models` | `opencode models` |
| Error classes in the stream | yes | — (text only) | auth only (unconf. beyond) | `error.type`, exit codes | `error.type` |
| Review detection | Skill tool `code-review` | unconf. | unconf. | — | `skill` tool |

## Codex CLI

rust-v0.172.0, read 2026-11-26.

1. **Headless stream.** `codex exec --json "<prompt>"` prints JSONL:
   `thread.started{thread_id}`, `turn.started`, `turn.completed{usage}`,
   `turn.failed{error{message}}`, `item.started|updated|completed{item}` and
   `error{message}`. Items are `agent_message`, `reasoning`,
   `command_execution` (status `in_progress|completed|failed|declined`,
   `exit_code`), `file_change`, `mcp_tool_call` (server, tool, arguments,
   result, error, status), `collab_tool_call`, `web_search`, `todo_list` and
   `error`. No single result event, no cost, no denial list;
   `-o/--output-last-message` writes the last message.
   [codex-rs/exec/src/exec_events.rs](https://github.com/openai/codex/blob/rust-v0.172.0/codex-rs/exec/src/exec_events.rs),
   [non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode).
2. **Session ID and resume.** The caller can't choose it: `exec` has no
   session flag ([codex-rs/exec/src/cli.rs](https://github.com/openai/codex/blob/rust-v0.172.0/codex-rs/exec/src/cli.rs))
   and the app-server's `ThreadStartParams` has no ID. The Factory captures
   `thread.started.thread_id`, a UUID, the first event. `codex exec resume
   <id> "<prompt>"` resumes headlessly; `--last` picks the newest in the
   working directory; `codex exec fork` branches one.
3. **System prompt.** Config `developer_instructions` appends to the built-in
   prompt and `model_instructions_file` replaces it; `-c
   developer_instructions="…"` sets it for one run. Whether a resume keeps it
   is unconfirmed, so a Harness passes it on every Turn.
   [Config reference](https://learn.chatgpt.com/docs/config-file/config-reference).
4. **Permissions.** `exec` runs with approval policy Never and a read-only
   sandbox ([codex-rs/exec/src/lib.rs](https://github.com/openai/codex/blob/rust-v0.172.0/codex-rs/exec/src/lib.rs));
   `--sandbox workspace-write` and `--add-dir` widen it. An approval request
   is rejected, not asked, and shows as a `command_execution` with status
   `declined`; there's no list of them. Pre-allowing is by
   `.codex/rules/*.rules` (`prefix_rule(pattern=[…], decision="allow")`) in
   a trusted project and, for MCP tools, `tools.<tool>.approval_mode`; how
   rules interact with Never in `exec` is unconfirmed.
   `--dangerously-bypass-approvals-and-sandbox` allows everything.
   [Rules](https://learn.chatgpt.com/docs/agent-configuration/rules).
5. **MCP.** No `.mcp.json`. `.codex/config.toml` takes
   `[mcp_servers.<name>]` with `url`, `http_headers`,
   `bearer_token_env_var`, `enabled_tools`, `required = true` and timeouts.
   Project config loads only in a trusted project: `codex exec --trust`.
   Tools are listed at thread start; deferral isn't documented.
   [MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).
6. **Hooks.** `.codex/hooks.json` or `[hooks]` in config: SessionStart,
   PreToolUse, PermissionRequest, PostToolUse, Stop and more, matched by a
   regex on `tool_name` (MCP tools are `mcp__<server>__<tool>`, as in Claude
   Code). PreToolUse answering `permissionDecision: "allow"` with
   `updatedInput` replaces the arguments; PostToolUse covers MCP tools. Stdin
   carries `session_id`, `transcript_path`, `cwd`, `hook_event_name`,
   `model` and `turn_id`. Hooks need trusting by hash, or
   `--dangerously-bypass-hook-trust`. [Hooks](https://learn.chatgpt.com/docs/hooks).
7. **Skills.** `.agents/skills/<name>/SKILL.md`, invoked as `$<name>`
   (the `$` sigil in `codex-rs/core/src/mentions.rs`); `exec` very likely
   honours it, unconfirmed. No project slash commands.
   [Build skills](https://learn.chatgpt.com/docs/build-skills).
8. **Subagents.** Built-in `default`, `worker` and `explorer`; custom ones
   in `.codex/agents/*.toml` with their own model and effort. They show as
   `collab_tool_call` items.
   [Subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents).
9. **Usage and limits.** `turn.completed.usage` has `input_tokens`,
   `cached_input_tokens`, `cache_write_input_tokens`, `output_tokens` and
   `reasoning_output_tokens`, as the thread's running total, not the turn's
   (`usage_from_last_total` in `event_processor_with_jsonl_output.rs`). No
   rate-limit event in `exec`; the app-server's account rate-limit
   notifications are unconfirmed.
10. **Model and effort.** `-m/--model`; effort is `-c
    model_reasoning_effort=<level>` with levels none, minimal, low, medium,
    high, xhigh, max, ultra and persistent
    (`codex-rs/protocol/src/openai_models.rs`); which a model takes varies.
    `codex debug models` lists models; the app-server's `model/list` gives
    each one's `supportedReasoningEfforts`.
11. **Errors.** `turn.failed` and `error` carry text only; the internal
    classes (`UsageLimitExceeded`, `ServerOverloaded`, `ModelNotFound`,
    `Unauthorized`, `HttpConnectionFailed`) don't reach the JSONL. The texts
    are fixed in `codex-rs/protocol/src/error.rs` ("Selected model is at
    capacity…", "Quota exceeded…", "rate limit exceeded…"), so a Harness
    matches text. Exit code 1 for every failure. The login and unknown-model
    texts are unconfirmed.

Also: `codex app-server` (JSON-RPC, experimental) answers approvals and
lists models, but still assigns the thread ID. Session logs are
`~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<thread_id>.jsonl`. The
instruction file is `AGENTS.md`.

## Copilot CLI

`copilot` v1.0.116, read 2026-11-26.

1. **Headless stream.** `copilot -p "<prompt>" --output-format json` prints
   JSONL. GitHub doesn't document the event schema. A third-party parser
   ([ralph-adapters copilot_stream.rs](https://docs.rs/crate/ralph-adapters/2.9.3/source/src/copilot_stream.rs))
   reads `assistant.message{content, toolRequests}`, `assistant.turn_start|end`,
   `tool.execution_start{toolCallId, toolName, arguments}`,
   `tool.execution_complete{success, result, error{message, code}}` and a
   final `result{exitCode, usage{sessionDurationMs, totalApiDurationMs}}`.
   The changelog adds session events (1.0.108) and a result record even on
   an early or login failure (1.0.109, 1.0.110). Whether `result` names the
   session is unconfirmed ([copilot-cli#52](https://github.com/github/copilot-cli/issues/52)).
2. **Session ID and resume.** `--session-id <uuid>` creates a session with
   the caller's ID; it can't be combined with `--resume`, `--continue` or
   `--connect`. `--resume=<id>` resumes under `-p` (1.0.110). Whether
   `--session-id` on an existing ID resumes it is unconfirmed, so a Harness
   uses `--resume` for later Turns.
   [Command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference).
3. **System prompt.** No flag: the programmatic reference leaves system
   prompt options out. Instructions come from files re-read every run
   (`AGENTS.md`, `CLAUDE.md`, `.github/copilot-instructions.md`,
   `.github/instructions/**/*.instructions.md`, and folders in
   `COPILOT_CUSTOM_INSTRUCTIONS_DIRS`), or from a custom agent chosen with
   `--agent`. The SDK's `systemMessage` appends, and is passed again on
   resume ([session persistence](https://docs.github.com/en/copilot/how-tos/copilot-sdk/use-copilot-sdk/session-persistence)).
   A Harness writes the Claim's system prompt to a file in a folder named by
   `COPILOT_CUSTOM_INSTRUCTIONS_DIRS` for each Turn.
4. **Permissions.** Under `-p` a permission request is denied when no flag
   allows it. `--allow-tool` and `--deny-tool` take `Kind(arg)` rules
   (`shell(git:*)`, `write`, `read`, `url`, or an MCP server as
   `code-central(claim)`), deny winning even over `--allow-all`; also
   `--allow-url`, `--add-dir` and `--no-ask-user`. Rules are per run, so a
   Harness passes the pre-allows on each resume. How a denial shows in the
   stream is unconfirmed (presumably a failed `tool.execution_complete`).
5. **MCP.** Reads the workspace `.mcp.json` and `.github/mcp.json`, with
   `type: "http"`, `url`, `headers`, `tools` and `deferTools` (`auto`, the
   default, defers them behind a tool search since 1.0.111). Under `-p`
   workspace MCP is skipped in an untrusted folder; `--trust-folder` can't be
   combined with `--session-id` or `--resume`, so a Harness sets
   `GITHUB_COPILOT_PROMPT_MODE_WORKSPACE_MCP=true` (1.0.112) or passes
   `--additional-mcp-config @<file>`, which needs no trust.
6. **Hooks.** `.github/hooks/*.json`, and the repo's `.claude/settings.json`
   too. Events include preToolUse, postToolUse and permissionRequest;
   PascalCase event names get the Claude-style payload (`tool_name`,
   `tool_input`, `session_id`). preToolUse returns `modifiedArgs`; the docs
   warn hook output can be dropped in some runtimes. Under `-p` repo hooks
   need trust or `GITHUB_COPILOT_PROMPT_MODE_REPO_HOOKS=true`.
   [Hooks configuration](https://docs.github.com/en/copilot/reference/hooks-configuration).
7. **Skills.** `SKILL.md` in `.github/skills`, `.agents/skills` or
   `.claude/skills`; `.claude/commands` is read too. Whether `-p "/<skill>
   …"` runs the skill is unconfirmed.
8. **Subagents.** Built-in code-review, explore, general-purpose, research,
   rubber-duck, security-review and task; custom ones in `.github/agents` or
   `.claude/agents`.
9. **Usage and limits.** The SDK's `assistant.usage` has input, output,
   reasoning, cache read and cache write tokens, cost and quota snapshots
   ([streaming events](https://docs.github.com/en/copilot/how-tos/copilot-sdk/features/streaming-events)),
   but the CLI's `-p` JSON is only confirmed to carry durations. Rate limits
   and quota show as errors (`rate_limit`, `quota`), not a running figure.
10. **Model and effort.** `--model` (default `auto`), `COPILOT_MODEL`, and
    `--effort` low, medium, high, xhigh or max. No command lists models.
11. **Errors.** `-p` exits 1 when interrupted or when the final answer is
    empty. A login failure still writes a `result`, details on stderr. The
    SDK's `session.error` has `errorType` authentication, quota or
    rate_limit with `statusCode`; the CLI's shape for an unknown model or a
    5xx is unconfirmed.

Also: the official SDK (Node, Python, Go and more) drives `copilot-runtime
--server` over JSON-RPC, with a caller-chosen `sessionId`, resume and
`systemMessage` ([github/copilot-sdk](https://github.com/github/copilot-sdk));
it may suit a Harness better than `-p`. Sessions live in
`~/.copilot/session-state/<id>/`. Login is a token in `COPILOT_GITHUB_TOKEN`,
`GH_TOKEN` or `GITHUB_TOKEN`.

## Gemini CLI

v0.71.1, read 2026-11-26.

1. **Headless stream.** `gemini -p "<prompt>" --output-format stream-json`:
   `init{session_id, model}`, `message{role, content, delta}`,
   `tool_use{tool_name, tool_id, parameters}`, `tool_result{tool_id, status,
   output, error{type, message}}`, `error{severity, message}` and
   `result{status, error, stats}`.
   [packages/core/src/output/types.ts](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/output/types.ts),
   [docs/cli/headless.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/headless.md).
2. **Session ID and resume.** `--session-id <id>` (letters, digits, `-`,
   `_`) is the caller's, and errors if it exists ("Use --resume").
   `--resume <id>` works headlessly: `nonInteractiveCli.ts` wires it, though
   the headless docs don't say so. Sessions are kept per project, by a hash
   of the working directory.
   [packages/cli/src/config/config.ts](https://github.com/google-gemini/gemini-cli/blob/main/packages/cli/src/config/config.ts),
   [docs/cli/session-management.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/session-management.md).
3. **System prompt.** No append. `GEMINI_SYSTEM_MD=<file>` replaces the
   whole prompt (`GEMINI_WRITE_SYSTEM_MD` exports the default to start
   from), and `GEMINI.md` context files load every run; `context.fileName`
   can name extra ones. A Harness writes the Claim's prompt to a context
   file each Turn rather than replace the built-in prompt.
   [docs/cli/system-prompt.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/system-prompt.md).
4. **Permissions.** `--approval-mode default|auto_edit|yolo|plan`; headless
   treats "ask" as deny. A denial is a `tool_result` with status error and
   `error.type` `policy_violation`; no list. Pre-allowing is a policy TOML
   passed with `--policy <file>` (`[[rule]]` with `toolName`, `mcpName`,
   `commandPrefix`, `decision = "allow"`, `priority`); workspace policies are
   off by default. `--allowed-tools` is deprecated.
   [docs/reference/policy-engine.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/policy-engine.md).
5. **MCP.** `.gemini/settings.json` `mcpServers.<name>` with `httpUrl`
   (streamable HTTP), `headers`, `timeout`, `trust` and `includeTools`. In an
   untrusted folder project settings don't load and headless exits 55
   (`FatalUntrustedWorkspaceError`); `--skip-trust` or
   `GEMINI_CLI_TRUST_WORKSPACE=true` passes it. Tools are named
   `mcp_<server>_<tool>` and all load at startup.
   [docs/tools/mcp-server.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md),
   [docs/cli/trusted-folders.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/trusted-folders.md).
6. **Hooks.** `hooks` in settings: BeforeTool, AfterTool, SessionStart and
   more, matched by regex. BeforeTool returns `hookSpecificOutput.tool_input`,
   merged over the arguments; AfterTool runs after every call. Stdin carries
   `session_id`, `cwd`, `tool_name` and `tool_input`; the env has
   `GEMINI_SESSION_ID` (and, for compatibility, `CLAUDE_PROJECT_DIR`).
   [docs/hooks/reference.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/hooks/reference.md).
7. **Skills.** Custom commands in `.gemini/commands/**/*.toml` run headlessly
   as `-p "/<name> <args>"` (`nonInteractiveCliCommands.ts`). Agent skills in
   `.gemini/skills` or `.agents/skills` load through an `activate_skill` tool
   that asks for consent; headless consent is unconfirmed. A Harness ships one
   command per Stage skill that activates it.
   [docs/cli/custom-commands.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/custom-commands.md),
   [docs/cli/skills.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/skills.md).
8. **Subagents.** Built-in codebase_investigator, cli_help and generalist;
   custom ones in `.gemini/agents/*.md`. `--model` doesn't reach them.
   [docs/core/subagents.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/core/subagents.md).
9. **Usage and limits.** Only `result.stats`: total, input, output and
   cached tokens, duration, tool calls, and a per-model breakdown. No cache
   write figure, no rate-limit event.
   [docs/cli/token-caching.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/token-caching.md).
10. **Model and effort.** `-m/--model`, `GEMINI_MODEL` or `model.name`
    (default `auto`, which routes between models); `--list-models`. No effort
    flag: thinking budget and level are per model in settings `modelConfigs`.
    [docs/cli/generation-settings.md](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/generation-settings.md).
11. **Errors.** Exit codes 0, 1 (general or API), 41 (login), 42 (input), 52
    (config), 53 (turn limit), 54 (tool), 55 (untrusted) and 130
    (cancelled) (`packages/core/src/utils/errors.ts`); fatal errors also
    write `result{status: "error", error{type, message}}`. An unknown model
    is `ModelNotFoundError` "Model not found" (exit code unconfirmed, likely
    1); quota is `TerminalQuotaError` or `RetryableQuotaError`. Whether
    headless silently falls back to another model on quota is unconfirmed.

Also: `gemini --acp` speaks the Agent Client Protocol. Session logs are JSON
in `~/.gemini/tmp/<project hash>/chats/`. The instruction file is
`GEMINI.md`.

## OpenCode

v2.4.2, read 2026-11-26.

1. **Headless stream.** `opencode run "<message>" --format json` prints
   JSONL, each line `{type, timestamp, sessionID, …}`: `step_start`, `text`,
   `reasoning`, `tool_use{part{tool, state{status, input, output|error}}}`,
   `step_finish{reason, cost, tokens}`, `retry`, `background_wait` and
   `error`. No final result: success is exit 0, failure a last `error` and
   exit 1. `opencode serve` offers the same over HTTP and SSE.
   [packages/cli/src/run/noninteractive.ts](https://github.com/anomalyco/opencode/blob/dev/packages/cli/src/run/noninteractive.ts),
   [server docs](https://opencode.ai/docs/server/).
2. **Session ID and resume.** `-s/--session <id>` continues that session or
   creates it when missing (`session-target.ts`), so one flag serves both.
   IDs must start `ses` (`packages/schema/src/session-id.ts`): the Factory
   would choose `ses_<uuid>`.
3. **System prompt.** No flag. `instructions` in `opencode.json` and
   `AGENTS.md` (falling back to `CLAUDE.md`) are appended, re-read from
   config every run; an agent's `system` replaces the prompt.
   [Rules](https://opencode.ai/docs/rules/).
4. **Permissions.** `permissions` is a list of `{action, resource, effect}`
   with effect allow, ask or deny, the last match winning; actions include
   read, edit, shell, skill, webfetch, external_directory and MCP
   `<server>_<tool>`. `run` rejects an ask ("This non-interactive run cannot
   ask…") unless `--auto`. A denial is a `tool_use` error of type
   `permission.rejected`; no list. Pre-allowing is config, not a flag.
   [Permissions](https://opencode.ai/docs/permissions/).
5. **MCP.** `opencode.json` `mcp.servers.<name>` with `type: "remote"`,
   `url`, `headers` and `oauth: false` (OAuth is on by default). No trust
   gate found. MCP tools go through Code Mode by default, one `execute` and
   one `search` tool, which defers them; `codemode: false` exposes each as
   `<server>_<tool>`.
   [MCP servers](https://opencode.ai/docs/mcp-servers/).
6. **Hooks.** Only TypeScript plugins in `.opencode/plugins/`:
   `ctx.tool.hook("execute.before")` sees `{tool, sessionID, input}` and may
   change `input`; `execute.after` fires on every call, Code Mode included.
   No shell-command hooks.
   [packages/plugin/src/effect/tool.ts](https://github.com/anomalyco/opencode/blob/dev/packages/plugin/src/effect/tool.ts),
   [plugins](https://opencode.ai/docs/build/plugins/server/).
7. **Skills.** Commands in `.opencode/commands/**.md`; skills in
   `.opencode/skills`, `.agents/skills` or `.claude/skills`, through a `skill`
   tool. v2's `run` neither parses a slash nor has v1's `--command`; the
   server's `POST /api/session/{id}/command` runs one. A Harness asks for the
   skill in words ("Use the implement skill on …") or drives the server.
   [Commands](https://opencode.ai/docs/commands/), [skills](https://opencode.ai/docs/skills/).
8. **Subagents.** Primary agents build and plan; subagents general and
   explore, through a `subagent` tool, each in a child session.
9. **Usage and limits.** Each `step_finish` has `cost` in USD and
   `tokens{input, output, reasoning, cache{read, write}}`
   (`packages/schema/src/token-usage.ts`). Many providers, so no plan-limit
   event; rate limits and quota arrive as errors with `retryAfterMs`.
10. **Model and effort.** `--model provider/model#variant`, where a variant
    (say `anthropic/claude-sonnet-5-5#high`) is configured per model with a
    reasoning effort. `opencode models` lists models; variants only via
    `GET /api/model`. [Models](https://opencode.ai/docs/models/).
11. **Errors.** `{"type": "error", "error": {type, message}}` then exit 1,
    with types `provider.auth`, `provider.rate-limit`, `provider.internal`,
    `provider.transport`, `provider.timeout`, `provider.quota`,
    `provider.no-route` (an unknown model or variant), `permission.rejected`
    and `unknown` (`packages/core/src/session/to-session-error.ts`). `retry`
    events precede an outage error, up to ten tries, or an hour with no
    network (`model-retry.ts`).

Also: sessions are in SQLite (`opencode.db`); `opencode session export`
gives one as JSON. `run` reuses a background service unless `--standalone`.

## The cloud Copilot coding agent

Ruled out. It works [in the background in a GitHub Actions environment and
delivers a pull request per task](https://docs.github.com/en/copilot/concepts/agents/cloud-agent/about-cloud-agent).
It isn't a local process the Factory starts, streams and stops: it has no
headless run in a worktree, no resume the Factory drives, no stream of
events, and its work arrives as a pull request where a Claim's lands as a
local branch. Copilot CLI is the Copilot Harness.

## A Harness that assigns its own session ID

Today the Factory makes a UUID, claims with it as `session_id` and the
Resume Command, then calls `set_session` with the worktree once the claim
names the Ticket (`factory/run.go`, `startClaimed`). Codex assigns its own
ID, so the Claim's session is known only once the Turn starts.

It fits without a change to Code Central: `session_id` on `claim` is
optional, and `set_session` already names a Claim's session after the claim.

1. The Factory claims with no session ID and no Resume Command.
2. It starts the first Turn. The Harness reports the ID through a callback
   as soon as it reads it (`thread.started`, Codex's first event).
3. The callback records it in the Factory's state file and calls
   `set_session` with the ID, the worktree and the Harness's Resume Command.
   Later Turns resume that ID.
4. A Turn that ends before reporting an ID (a failed start, a stop) leaves
   the Claim with no session: the next Turn starts a new one, as a lost
   session does today. A Factory that restarts with a Claim and no session
   does the same.

The hooks need nothing: Codex's hook stdin carries `session_id`, so
`code-central hook session-id` fills `post_round` and `set_session` as it
does under Claude Code. Until step 3, the human sees the Claim without a
Resume Command, for the seconds the Turn takes to start.

## Couplings: conceptual or naming

"Naming" means the concept holds for every tool and only the flag, field or
word is Claude Code's, so the Harness renames it. "Conceptual" means at
least one tool lacks the concept, so it becomes a declared capability or a
different mechanism.

### Factory

| Coupling | Where | Verdict | Why |
|---|---|---|---|
| Executable lookup (`claude`) | `factory/factory.go` | naming | Each tool is one executable. |
| Windows shim refusal | `factory/harness/claude/shim_windows.go` `refuseShim` | naming | Copilot, Gemini and OpenCode install through npm as `.cmd` shims too; the check moves into the Harness's lookup, each naming its own. |
| argv: `-p`, `--output-format stream-json --verbose` | `claudeturn.go` `args` | naming | Every tool has a headless JSON mode. |
| Factory-chosen `--session-id` | `run.go`, `chat.go` | conceptual | Codex assigns it. |
| `--resume <id>` | `claudeturn.go` | naming | All resume by ID; OpenCode with the same flag that creates. |
| `--append-system-prompt`, relied on to persist across resume | `claudeturn.go`, `stage.go`, `run.go` | conceptual | Only Codex has a flag; the rest read instruction files. Passing it each Turn, by flag or file, works for all. |
| Permission flags (`--permission-mode`, `--permission-prompts none`, `--bypass-permissions`) | `claudeturn.go` `flags` | naming | Every headless mode refuses rather than asks; bypass exists in all. |
| `--allowedTools` pattern from a denial | `denials.go` | naming | Copilot `--allow-tool`, Gemini `--policy`, Codex rules, OpenCode config: each needs its own syntax, and Codex and OpenCode a file. |
| `permission_denials` list on `result` | `claudeturn.go` `result` | conceptual | No other tool lists denials; Codex, Gemini and OpenCode mark each denied call, so the Harness gathers them from the stream. |
| `--add-dir` | `claudeturn.go`, `chat.go` | naming | Codex and Copilot `--add-dir`, Gemini `--include-directories`, OpenCode `external_directory`. |
| `--model`, `--effort` | `modelchoice.go` | model naming, effort conceptual | Gemini has no effort flag; OpenCode's is part of the model. |
| `claude --help` discovery of aliases and efforts | `claudehelp.go` | conceptual | Codex and Gemini list models, OpenCode lists models, Copilot lists nothing; efforts come from different places or nowhere. |
| Claude Args and its reserved flags | `claudeargs.go` | naming | Each tool has extra args and its own reserved set; Claude Args keeps its name (out of scope). |
| `result` event and `turnEnd` | `claudeturn.go` | naming | Codex and OpenCode have no result event; the Harness builds the same end from `turn.completed`/`turn.failed` or the last `step_finish` and exit code. |
| API error classes (`authentication_failed`, `model_not_found`, outage set) | `claudeturn.go` `apiError` | naming | Every tool signals login, unknown model and outage; Codex only as text. |
| `system/api_retry` as an outage signal | `claudeturn.go` | naming | Codex text, OpenCode `retry` events, Gemini `RetryableQuotaError`. |
| Hook refusal detected by "PreToolUse:" | `claudeturn.go` `isHookRefusal` | conceptual | A Claude Code message format; the others report a hook block differently or not at all. |
| `rate_limit_event` and Plan Limits | `planlimits.go` | conceptual | No other tool reports plan utilisation. |
| Usage fields and cache tokens | `usage.go` | naming, with two differences | All report input, output and cache reads; Gemini lacks cache writes, Codex reports running totals, Copilot's CLI may report none. |
| Subagent attribution by `parent_tool_use_id` | `usage.go`, `events.go` | conceptual | Unconfirmed or absent elsewhere. |
| Session Events from tool names | `events.go` | naming | Each tool names its tools; the event kinds hold. |
| Review detection (Skill tool, `code-review`) | `reviewcheck.go` | conceptual | Needs a skill tool the stream shows; only OpenCode's `skill` tool is known to. |
| First prompt `/<skill> <ticket>` | `stage.go` `stageSkills` | conceptual | Codex `$<skill>`, Gemini a command, OpenCode words, Copilot unconfirmed. |
| ToolSearch note in the claim intro | `stage.go` | conceptual | Only Claude Code defers through ToolSearch; Copilot and OpenCode defer differently, Codex and Gemini not at all. |
| `.mcp.json` check on the base | `base.go` `checkMCP` | naming | Every tool has a project MCP file; Copilot reads the same one. |
| Trust of the repo's config | none today | conceptual (new) | Codex, Copilot and Gemini need a trust flag or env var before reading it; Claude Code needs none. |
| Resume Command strings | `run.go`, `chat.go`, `outcome.go` | naming | Every tool resumes from a command line. |
| Wording ("claude", "Claude Code") | `factory.go` usage and flags, `outcome.go`, `run.go`, `work.go` | naming | Takes the Harness's name. |
| Stop steps (SIGINT, SIGTERM, kill; Ctrl-Break) | `claudeturn.go`, `process_*.go` | naming | Generic process handling; OpenCode handles SIGINT cleanly, the others are unconfirmed but the kill step covers them. |
| Chat's Turn (`claude -p` with `haiku`) | `chat.go` | naming | Chat runs a Turn like any other; its default model is the Harness's. |

### Code Central

| Coupling | Where | Verdict | Why |
|---|---|---|---|
| Chat author `claude` | `api/chat.go` `ChatFromClaude` | naming | The reply comes from whichever agent; the author is "the agent". |
| Default Chat model `haiku` | `core/factories/chat.go` | naming | Each Harness has a cheap model. |
| Reported Models: aliases and efforts "its claude accepts" | `core/reportedmodels.go` | model naming, effort conceptual | Models hold everywhere; efforts don't exist in Gemini and differ elsewhere. |
| Seeded efforts low to max | `core/foundation/effort.go` | conceptual | Codex's are none to ultra, Gemini has none. |
| Plan Limits, "the Claude account's 5-hour and 7-day" | `core/factories/planlimits.go` | conceptual | Claude Code only. |
| Cache write and read tokens; `ContextSize` | `api/usage.go` | naming | Present in three tools; a zero where a tool lacks one is honest. |
| `CLAUDE_CODE_SESSION_ID` | `cli/session.go` | naming | Gemini sets `GEMINI_SESSION_ID`; Codex, Copilot and OpenCode pass it on hook stdin only. |
| `hook session-id` output (`hookSpecificOutput`, `updatedInput`) | `cli/session.go` | naming | Codex matches it; Copilot `modifiedArgs`, Gemini `tool_input`, OpenCode a plugin. |
| `ping` on PostToolUse | `cli/session.go` | naming | Every tool has an after-tool hook. |
| Hooks written to `.claude/settings.json` by `init` | `cli/session.go` | naming | Each tool has its file; OpenCode needs a TypeScript plugin. |
| `CLAUDE.md` pointer written by `init` | `cli/teaching.go` | naming | `AGENTS.md` serves Codex, Copilot and OpenCode; Gemini reads `GEMINI.md`. |
| `resume_command` description "claude --resume" | `api/claim.go` | naming | |
| Resume fallback `claude --resume ${sessionId}` | `web/src/panels/resume/ResumeCommand.tsx` | naming | |
| "Claude Code's default" | `web/src/format.ts`, `domain/domain.go` | naming | |
| "Ask Claude" | `web/src/chat` | naming | |
| Stage Guides naming the `code-review` skill and subagents | `core/stageguides/` | conceptual for review, naming for subagents | Every tool has subagents; a review skill each tool runs is a skill packaging question. |
| Context Report reading `~/.claude/projects` | `internal/contextreport` | conceptual | Claude-only, out of scope. |

## Recommended changes to Code Central

For when a second Harness exists; none made here.

- **Chat author.** Rename `ChatFromClaude` to an agent author, keeping the
  stored `claude` value readable, and show the Harness's name.
- **Plan Limits.** Keep the name (out of scope), but say "the account's
  limits, as the agent reports them" and show nothing, not zeros, when the
  Factory's Harness has no plan-limit events.
- **Cache tokens.** Keep both fields; document that a Harness without one
  reports zero, and that `ContextSize` then undercounts for Gemini.
- **Reported Models.** Let a Factory report "no efforts" so the picker hides
  effort; stop seeding Claude's efforts as if universal: seed per Harness, or
  only from what the Factory reports.
- **Hooks.** Give `code-central hook session-id` an output shape per tool
  (`--for codex|copilot|gemini`), read the session from stdin before
  `CLAUDE_CODE_SESSION_ID`, and ship an OpenCode plugin that calls the same
  commands.
- **`init`.** Take the Harness (`--harness`, default `claude`) and write that
  tool's hook file, MCP file and instruction file (`AGENTS.md` or
  `GEMINI.md`) instead of `.claude/settings.json`, `.mcp.json` and
  `CLAUDE.md`.
- **UI copy.** Have the Factory report its Harness's display name and use it
  for "Ask …", the default-model wording and the Resume Command fallback;
  until then, drop the fallback and show only the Resume Command set.
- **`resume_command` description.** Stop naming `claude --resume`.

## What each tool keeps instead of the Claude-only features

| Feature | Codex CLI | Copilot CLI | Gemini CLI | OpenCode |
|---|---|---|---|---|
| Context Report (`~/.claude/projects`) | `~/.codex/sessions/…/rollout-*.jsonl` with `turn.completed` usage | `~/.copilot/session-state/<id>/` and `--share` transcripts; usage via the SDK | `~/.gemini/tmp/<hash>/chats/*.json` with `result.stats` | `opencode session export` from SQLite, per-step tokens and cost |
| Retro session logs | the rollout file named by thread ID | the session-state folder | the chats folder, by project hash | `opencode session export <id>` |
| Manual flow (`docs/working-with-claude-code.md`) | `codex resume <id>` interactively, `AGENTS.md` | `copilot --resume`, `AGENTS.md` | `gemini --resume`, `GEMINI.md` | `opencode -s <id>`, `AGENTS.md` |
| Skills plugin (`mattpocock-skills`) | copy to `.agents/skills` | `.agents/skills` or `.claude/skills` | `.agents/skills` plus a command per skill | `.agents/skills` or `.claude/skills` |

`.agents/skills` is read by Codex, Copilot, Gemini and OpenCode, so one
copy of the skills there serves all four.

## Proposed Harness interface

A package `factory/harness` holds the interface and the neutral types;
`factory/harness/claude` holds the Claude Code implementation. One Harness
per Factory, chosen by `--harness` (default `claude`).

```go
type Harness interface {
	Name() string // "Claude Code", for wording and the Resume Command
	Start(ctx context.Context, args []string) (Setup, error)
	Run(ctx context.Context, t Turn) Outcome
	ResumeCommand(session string, addDirs []string) string
	SessionID() SessionIDs
	BaseFiles() []string
	FirstPrompt(skill, args string) string
	ToolLoadingNote() string
	Capabilities() Capabilities
}
```

- **Start** finds the executable (lookup and, on Windows, shim refusal),
  checks the Harness Args against its reserved flags, and discovers models
  and efforts into `Setup{Models, Efforts}`; an empty `Efforts` hides effort.
- **Run** takes today's `turnInput` minus the Claude names: the directory,
  extra directories, the session to start or resume, model and effort, the
  pre-allowed `Denial`s, the system prompt, the prompt, and the callbacks
  (events, usage, plan limits, lines). It adds `OnSession(id string)`,
  called once the session ID is known, before the Turn's first event.
- **Outcome** is the Spec's: a kind (done, failed, outage, login failed,
  model rejected, effort rejected, stopped), the exit description, the last
  text, the error class, the denials, whether a review ran, and the accepted
  efforts when an effort is rejected.
- **Denial** is `{Tool, Subject}`: the tool and what it was asked to do (a
  command, a path, a host). The Harness turns denials into its own
  pre-allows on resume, whether flags or a file it writes.
- **Usage** reaches `OnUsage` as a per-Turn delta; a Harness whose tool
  reports running totals (Codex) subtracts.
- **System prompt** is passed on every Turn, first and resumed; the Harness
  decides how (a flag, `-c`, or an instruction file it writes). Nothing
  relies on it persisting.
- **SessionIDs** is `FactoryChooses(new func() string)` or `HarnessAssigns`:
  the format differs (`ses_` for OpenCode, a UUID for Copilot), so the
  Harness makes the ID.
- **BaseFiles** are the files the base must hold: the MCP config and, where
  the tool reads them from the repo, the hook config.
- **FirstPrompt** runs a Stage skill (`/implement code-central-1`,
  `$implement …`, or words), and **ToolLoadingNote** is the claim intro's
  line, empty where tools aren't deferred.
- **Capabilities** declares `PlanLimits`, `Effort`, `DenialList`,
  `ReviewDetection` and `SubagentUsage`; the Factory never emulates a missing
  one.

### Checked against the matrix

| Method or field | Claude Code | Codex CLI | Copilot CLI | Gemini CLI | OpenCode |
|---|---|---|---|---|---|
| Start: lookup, args, discovery | `--help` | `codex debug models` | models unknown | `--list-models` | `opencode models` |
| Run → Outcome | `result` | `turn.*` and exit code | `result` | `result` and exit code | `step_finish`, `error`, exit code |
| Outcome kinds from errors | classes | text match | partly | types and exit codes | types |
| OnSession | at start (chosen) | `thread.started` | at start (chosen) | at start (chosen) | at start (chosen) |
| Denials | list | `declined` items | per tool (unconf.) | `policy_violation` results | `permission.rejected` |
| Pre-allows on resume | `--allowedTools` | rules file | `--allow-tool` | `--policy` file | config file |
| Usage delta | per message | difference of totals | none or SDK | `result.stats` | sum of steps |
| System prompt each Turn | flag | `-c` | instructions folder | context file | `instructions` config |
| SessionIDs | chooses | assigns | chooses | chooses | chooses |
| BaseFiles | `.mcp.json` | `.codex/config.toml` | `.mcp.json` | `.gemini/settings.json` | `opencode.json` |
| FirstPrompt | `/skill` | `$skill` | unconf. | `/command` | words |
| Capabilities | all | effort | effort | none | effort, review |

Every tool fills every method; the gaps fall on declared capabilities.

### Where it differs from the Spec's sketch

- **Session ID.** The sketch leaves room for a self-assigned ID; this makes
  it concrete as `SessionIDs` and `OnSession`, which Codex needs. Building
  it stays out of scope, but `OnSession` costs nothing for Claude Code: it
  calls it at once with the chosen ID.
- **The Harness makes the ID** even when the Factory chooses it, since
  OpenCode wants a `ses` prefix.
- **The system prompt goes with every Turn.** The sketch's "how the system
  prompt is passed" is right, but the Factory must stop relying on it
  persisting across a resume.
- **Usage is a delta**, normalised by the Harness.
- **Denials may be gathered from the stream**, not read from a list; the
  `DenialList` capability says whether a denial Round is possible at all.
- **BaseFiles replaces the single MCP config file**, since the hook config
  is a repo file for every tool and Codex, Copilot and Gemini read both only
  once the folder is trusted: the Harness passes its trust flag or env var
  on every Turn.
- **Two more capabilities**, `SubagentUsage` and the trust handling above,
  beyond the sketch's plan limits, effort, denial list and review detection.
- **Name()** feeds wording and the Resume Command, as the sketch's
  "Wording takes the Harness's name" asks.
