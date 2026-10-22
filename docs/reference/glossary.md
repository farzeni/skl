# Glossary

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

| Term | Example | What it is |
|---|---|---|
| **Skill** | `review/pr-review` | A folder with a `SKILL.md`, plus any files next to it. |
| **Group** | `review/` | A folder of skills in your vault. Loading a group loads all of them. |
| **Vault** | `github.com/you/skills` | Your git repo of skills, copies, lessons and skill sets. The only thing you edit. |
| **Selector** | `review`, `@backend` | One word on the command line that picks skills to load or remove. See [Selectors and references](./repos#selectors). |
| **Skill set** | `@backend` | A list of selectors you load together. What's loaded now is your active skill set; save it with a name to load it anywhere. See [Skill sets](../guide/skill-sets). |
| **Generation** | `#12` | A numbered snapshot taken after every change, so any change can be rolled back. See [Rollback](../guide/rollback). |
| **Copy** | `someone/pdf` | Someone else's skill copied into your vault, with its source recorded. See [Get one](../guide/other-skills#get-one). |
| **Source** | `someone/agent-skills/pdf` | The skill a copy was made from. `skl sync` checks it for updates. |
| **Peer** | `someone/agent-skills/deep-pr-review` | The skill a lesson learns from. |
| **Lesson** | peer → yours | A capability your skill learned from its peer, through an interview with Claude. See [Learn](../guide/learn). |
