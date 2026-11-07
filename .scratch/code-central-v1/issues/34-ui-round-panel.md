# 34: Round panel

**What to build:** The human selects a Ticket waiting on Questions and answers its Round in the inbox: every Question at once, each answer box pre-filled with the recommended answer, with Answer and Dismiss per Question and Accept all recommended. The waiting agent wakes when the Round is complete. Proved end to end in a real browser. See the [spec](../spec.md), section "UI".

**Blocked by:** 29, 33

**Status:** ready-for-agent

- [ ] The Round panel is a shared component, independent of the inbox layout
- [ ] It shows every Question of the open Round with its title and body; each answer box is pre-filled with the recommendation as a draft, labelled as such, and nothing is stored until submitted
- [ ] Answer and Dismiss per Question call `answer` and `dismiss_question`; Accept all recommended calls `accept_recommended`
- [ ] Answers save one by one and stay editable until the Round is complete
- [ ] The TypeScript API module and its fake gain `answer`, `accept_recommended` and `dismiss_question`, covered by the contract suite
- [ ] Component tests against the fake API check what the panel shows and which action it calls with what payload, including the pre-filled recommendation and Accept all recommended
- [ ] End-to-end test 3, Playwright against the embedded, built UI: an agent posts a Round, the inbox updates over SSE, and the human answers it
