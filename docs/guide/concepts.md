# Concepts

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

skl is built on six ideas.

| Concept | Example | What it is |
|---|---|---|
| **Skill** | `review/pr-review` | A folder with a `SKILL.md`, plus any files next to it. |
| **Group** | `review/` | A folder of skills. Loading a group loads all of them. |
| **Vault** | `github.com/you/skills` | Your repo of skills, forks, lessons and skill sets. The only thing you edit. |
| **Generation** | `#12` | A snapshot taken after every change, so any change can be rolled back. |
| **Lesson** | peer → yours | A capability your skill learned from another skill, through an interview with Claude. |
| **Skill set** | `@backend` | A list of groups and skills you load together. What's loaded now is your active skill set; save it with a name to load it anywhere. |

## How they fit together

Skills come from your vault or from other people's repos. skl reads them, works out which ones you asked for, and copies them into a target folder: `~/.claude/skills` by default. Changes made to those copies are saved back to your vault. Each change creates a generation. Forks and lessons are written into your vault, so they travel with it to every machine.

skl never edits other people's repos. It doesn't touch skills it didn't create either.

Everything you load is a [skill set](./skill-sets): you can [sync](./sync) it, [roll it back](./rollback), and copy someone else's with [`skl learn`](./learn).
