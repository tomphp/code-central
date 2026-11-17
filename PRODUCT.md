# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

A solo developer driving several Claude Code sessions at once, often across several Projects, each with its own Factory running. They are usually doing something else and glance at Code Central from another window to answer one Round, triage an Incoming Issue, or accept finished work, then go back to what they were doing. Developers like them who install it from the README are the same audience; there is no team or multi-user role.

## Product Purpose

Code Central is a local coordination point for AI agent-driven software delivery across many repositories. It holds the work (Requirements, Specs, Tickets, Maps) and its state through a fixed workflow of Stages, and shows the human what needs them. A Requirement goes through grilling and spec Stages, then either a tickets Stage that splits the Spec into Tickets or Implement, which builds it as one Ticket, and accepted work lands by the Project's Landing Method.

Success is the human spending their attention only where it is needed: every question for them arrives as a Round, answered one at a time, while the rest runs unattended to landed code.

## Positioning

One question at a time. Needs Me and Focus Mode surface only what needs the human across many concurrent agent sessions, and everything else moves through fixed Stages to landed code without them. A kanban board shows all the work; a plain Claude Code session asks its questions in its own terminal; Code Central gathers every session's questions into one queue.

## Operating Context

- Runs as a local service at `http://localhost:7733/`, started with `code-central serve` or installed as a service; the UI is served by it.
- A Factory (`code-central-factory`, or one the user writes) runs per Project in its Home Repository, claims Tickets and runs Claude Code sessions on them, sending Heartbeats that show as the Project's Factory Status dot.
- Built around the Matt Pocock Skills workflow (grilling, spec, tickets, implement, wayfinder, prototype, retro).
- Work spans a Project's Repositories, lands in their landing order, and can be gated on CI (GitHub Actions, Azure Pipelines) or go through pull requests (GitHub, Azure Repos). GitHub issues can come in as Incoming Issues to triage.
- Claude Code sessions also reach Code Central through its MCP server and CLI, so the UI is one of several clients of the same HTTP API.

## Capabilities and Constraints

- Local only: runs on the user's own machine at localhost. No accounts, no hosted SaaS, no marketing surface.
- The UI is the human's surface: Inbox and All Tickets view, Needs Me, Focus Mode, Spec and Ticket pages, Maps, the Activity Dashboard, Project Options, Options, and Chat with a Factory that supports it.
- The service is the only writer (ADR 0002); the UI is a client of the HTTP API, with wire types generated from Go's view structs (ADR 0042).
- The domain language in `GLOSSARY.md` is binding in the UI's copy: Project, Repository, Spec, Ticket, Stage, Round, Claim, Factory, Landing Method and the rest, avoiding the synonyms it lists.
- Assumes macOS first; Windows is partly supported.

## Brand Commitments

- Name: Code Central.
- Voice: plain and honest. The README describes it as "a vibe coded experiment for personal use", and the product must keep that honesty: never claim users, scale, polish or guarantees it doesn't have.

## Evidence on Hand

- The README, the guides in `docs/` and the ADRs in `docs/adr/` show real commands and output.
- There are no users, testimonials, metrics, customers or press, and none should be invented.

## Product Principles

1. One question at a time: the human's attention is the scarce resource; show what needs them and keep everything else quiet.
2. Glanceable state: a returning user should see at a glance what changed, what is waiting on them and what is stuck.
3. The domain's own words: name things exactly as `GLOSSARY.md` does, in the UI as in the code.
4. Honest about what it is: an experimental, local, personal tool; no overclaiming.

## Accessibility & Inclusion

- Light and dark themes both stay fully working: the UI follows `prefers-color-scheme`, with a `data-theme` override.
- Fully keyboard-operable, with WCAG AA contrast in both themes.
