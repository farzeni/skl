# Concepts

skl is built on six ideas.

| Concept | Example | What it is |
|---|---|---|
| **Skill** | `review/pr-review` | A folder with a `SKILL.md`, plus any files next to it. |
| **Group** | `review/` | A folder of skills. Loading a group loads all of them. |
| **Vault** | `github.com/you/skills` | Your repo of skills, forks, lessons and profiles. The only thing you edit. |
| **Profile** | `@backend` | A saved mix of groups and skills, loaded in one go. |
| **Generation** | `#12` | A snapshot taken after every change, so any change can be rolled back. |
| **Lesson** | peer → yours | A capability your skill learned from another skill, through an interview with Claude. |

## How they fit together

Skills come from your vault or from other people's repos. skl reads them, works out which ones you asked for, and links them into a target folder: `~/.claude/skills` by default. Each change creates a generation. Forks and lessons are written into your vault, so they travel with it to every machine.

skl never edits other people's repos. It doesn't touch skills it didn't create either.
