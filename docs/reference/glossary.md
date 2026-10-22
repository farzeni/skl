# Glossary

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

| Term | Example | What it is |
|---|---|---|
| **Skill** | `review/pr-review` | A folder with a `SKILL.md`, plus any files next to it. |
| **Group** | `review/` | A folder of skills in your vault. Loading a group loads all of them. |
| **Vault** | `github.com/you/skills` | Your git repo of skills and skill sets, including the ones you learned. The only thing you edit. |
| **Selector** | `review`, `@backend` | One word on the command line that picks skills to load or remove. See [Selectors and references](./repos#selectors). |
| **Skill set** | `@backend` | A list of selectors you load together. What's loaded now is your active skill set; save it with a name to load it anywhere. See [Skill sets](../guide/skill-sets). |
| **Generation** | `#12` | A numbered snapshot taken after every change, so any change can be rolled back. See [Rollback](../guide/rollback). |
| **Source** | `someone/agent-skills/pdf` | Where a skill you learned without Claude came from, as recorded when it was copied. `skl sync` checks it for updates. See [Get a skill](../guide/learn#get-a-skill). |
| **Update** | `update available` | A later version of a skill's source or peer, found by `skl sync` and taken with `skl learn <skill>`. See [Keep up with its author](../guide/learn#keep-up-with-its-author). |
| **Peer** | `someone/agent-skills/deep-pr-review` | The skill a lesson learns from. |
| **Lesson** | peer → yours | A capability your skill learned from its peer, through an interview with Claude. See [Learn](../guide/learn). |
