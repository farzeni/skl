# Glossary

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

| Term | Example | What it is |
|---|---|---|
| **Skill** | `review/pr-review` | A folder with a `SKILL.md`, plus any files next to it. |
| **Group** | `review/` | A folder of skills in your vault. Loading a group loads all of them. |
| **Vault** | `github.com/you/skills` | Your git repo of skills and skill sets, including the ones you learned. The one place your skills are kept. |
| **Loaded** | `~/.claude/skills/pr-review` | A skill skl copied from your vault to where your agent reads it, such as Claude Code. You can keep editing it there; skl saves your changes back. See [Your skills](../guide/skills). |
| **Scope** | `--scope project` | Where skills are loaded: the user scope `~/.claude/skills` (the default), or a repo's project scope `./.claude/skills`. |
| **Target** | `agents` | A pair of folders skl loads into, one per scope: `claude` (`~/.claude/skills`, the default) or `agents` (`~/.agents/skills`). See [Which agents read them](../guide/skill-sets#which-agents-read-them). |
| **Selector** | `review`, `@backend` | One word on the command line that picks skills to load or remove. See [Selectors and references](./repos#selectors). |
| **Reference** | `someone/agent-skills/pdf` | A skill, a skill set or a repo in someone else's git remote. See [Repos](./repos#repos). |
| **Skill set** | `@backend` | A list of selectors you load together. See [Skill sets](../guide/skill-sets). |
| **Active skill set** | `@backend writing/changelog` | The selectors loaded now in a scope. |
| **Named skill set** | `sets/backend.toml` | A skill set saved with `skl save`, loaded anywhere with `@name`. |
| **Generation** | `#12` | A numbered snapshot taken after every change, so any change can be rolled back. Numbers are per machine. See [Rollback](../guide/rollback). |
| **Author** | `someone` | Whoever owns the repo a skill came from. Their later changes are its updates. |
| **Copy** | `someone/pdf` | A skill learned without a lesson: their folder, copied as it is. |
| **Source** | `someone/agent-skills/pdf` | Where a copy came from, as recorded when it was copied. `skl sync` checks it for updates. See [Get a skill](../guide/learn#get-a-skill). |
| **Pin** | `#v1.2` | A tag, branch or commit after `#` in a reference. A tag or commit never updates; a branch is followed. See [Pinning a version](./repos#pinning-a-version). |
| **Update** | `update available` | A later version of a skill's source or peer, found by `skl sync` and taken with `skl learn <skill>`. See [Keep up with its author](../guide/learn#keep-up-with-its-author). |
| **Peer** | `someone/agent-skills/deep-pr-review` | The skill a lesson learns from. |
| **Lesson** | peer → yours | One run of `skl learn` with the engine: Claude interviews you and rewrites your skill with what it takes from the peer. See [Learn](../guide/learn). |
| **Engine** | `claude` | The agent that runs lessons and sync conflicts: the `claude` CLI by default, or a command you set. See [Other agents](../guide/learn#other-agents). |
