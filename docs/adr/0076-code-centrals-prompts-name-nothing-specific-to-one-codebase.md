# Code Central's prompts name nothing specific to one codebase; what a Project needs comes from a named Project setting

The Factory's conflict prompt told the agent to run `make typecheck` after resolving a merge or rebase. That is Code Central's own command: a Project whose Repositories check their code another way, such as `npm run check`, was told to run a command it doesn't have. Code Central serves many Projects, so a prompt that names one codebase's tooling is wrong for the others.

ADR 0059 lets a Stage Guide vary by Project only through named Project settings that the human sets. This extends that rule to every prompt Code Central writes, the Factory's own prompts such as the conflict prompt included: a prompt names nothing specific to one codebase, and what a Project needs in it comes from a named Project setting.

The first case is the **Check Command** (Spec code-central-1229), set for the Project as the default and for a Repository as its own, by the human alone, never by an agent. A Repository's Check Command is its own, else the Project's, else none. When it has none, the prompt falls back to the Repository's own docs, such as CLAUDE.md or README, rather than naming a guess.

## Considered Options

- **Keeping `make typecheck` as a default.** Rejected: it is a guess right for one codebase only, and a wrong command costs a Claim more than no command.
- **A free-text Project prompt addition.** Rejected: speculative, and it would let a Project rewrite a prompt's meaning, where a named setting fills in one thing the prompt asks for.
- **Asking the agent to discover the command every time.** Kept as the fall-back only: the Repository's docs are the right source when the human has named nothing, but a named command is quicker and the same on every Claim.

## Consequences

- A prompt that needs something about a Project's codebase gains a named Project setting, with an empty value that falls back to the Repository's own docs.
- Existing Projects read with no Check Command, so their prompts point to the Repository's docs until the human sets one; Code Central's own Project sets `make typecheck` as a setting, not in code.
- A change to the setting applies from the next prompt Code Central writes, without a restart.
