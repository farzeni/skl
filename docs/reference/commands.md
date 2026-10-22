# Commands

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Wherever a command takes `<selectors>`, `<repo>` or `<ref>`, any form from [Selectors and references](./repos) works.

## Start

| Command | Description |
|---|---|
| `skl init <repo>` | Clone your vault, or set up a new one. |
| `skl import <name> [group]` | Copy a skill skl didn't create into your vault; the original becomes skl's copy. See [Add a skill you made](../guide/skills#add-a-skill-you-made). |

## Load

| Command | Description |
|---|---|
| `skl use <selectors>` | Replace the active skill set. See [selectors](./repos#selectors). |
| `skl add <selectors>` | Add to the active skill set. |
| `skl drop <selectors>` | Remove from the active skill set. |
| `skl status` | Show what's loaded, where each skill came from, your named skill sets, changes not saved back yet, selectors that match nothing, and updates that `skl sync` found. |
| `skl save <name>` | Save the selectors behind the active skill set as a named skill set. Asks before overwriting one. See [Named skill sets](../guide/skill-sets#named-skill-sets). |

## Undo

| Command | Description |
|---|---|
| `skl history` | List generations. |
| `skl rollback [N]` | Go back one generation, or to generation `N`. See [Rollback](../guide/rollback). |

## Other people's skills and learn

| Command | Description |
|---|---|
| `skl learn <peer> [skill] ["<text>"]` | Copy a skill or skill set you don't have, or with text build a new skill from it. Improve a skill you have with what its peer does better. With only your skill, take its author's changes: as they are if you never changed it, else by a lesson. With no arguments, take every available update; `--dry-run` lists them. See [Other people's skills](../guide/other-skills) and [Learn](../guide/learn). |
| `skl open [ref]` | Open a skill, skill set or repo in your browser, or your vault without `ref`. Always prints the URL. See [Web pages](./repos#web-pages). |

## Sync

| Command | Description |
|---|---|
| `skl sync` | Commit hand edits, pull, push, reload what's loaded, and check the skills you learned for updates. On a conflict, resolve with Claude, by hand or abort; `--continue` and `--abort` finish or undo a merge. See [Sync](../guide/sync). |

Optional [third-party parts](../thirdparty/) add more, such as `skl search`.

## Global flags

| Flag | Effect |
|---|---|
| `--scope user\|project` | Where to load skills. `user` is `~/.claude/skills`; `project` is `./.claude/skills`. Defaults to `scope` in the [config](./config), else `user`. |
| `--dry-run` | Show what would change, and change nothing. |
| `--yes` | Answer yes to confirmations, such as `Add it?` before someone else's skill lands. |
