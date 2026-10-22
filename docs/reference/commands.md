# Commands

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Wherever a command takes `<selectors>`, `<repo>` or `<ref>`, any form from [Selectors and references](./repos) works.

## Start

| Command | Description |
|---|---|
| `skl init <repo> [selectors]` | Clone your vault, or set up a new one, and load `selectors` if given. Offers to import the skills in `~/.claude/skills` that aren't in a vault. See [Getting started](../guide/getting-started#make-your-vault). |
| `skl import <name>… [--group <group>]` | Copy skills skl didn't create into your vault; each original becomes the loaded skill. The group defaults to your vault's owner (`you` for `you/skills`). See [Add a skill you made](../guide/skills#add-a-skill-you-made). |

## Load

| Command | Description |
|---|---|
| `skl use <selectors>` | Replace the active skill set. See [selectors](./repos#selectors). |
| `skl add <selectors>` | Add to the active skill set. |
| `skl drop <selectors>` | Remove from the active skill set. |
| `skl save <name>` | Save the selectors behind the active skill set as a named skill set. Asks before overwriting one. See [Named skill sets](../guide/skill-sets#named-skill-sets). |
| `skl status` | Show what's loaded, where each skill came from, your named skill sets, and everything that needs you: changes not saved back yet, skills not in your vault, selectors that match nothing, updates that `skl sync` found, and changes not pushed. Each such line ends with the command to run. |

## From others

| Command | Description |
|---|---|
| `skl learn <peer> [skill] ["<text>"]` | Copy a skill or skill set you don't have, or with text build a new skill from it. Improve a skill you have with what its peer does better. With only your skill, take its author's changes: as they are if you never changed it, else by a lesson. With no arguments, take every available update; `--dry-run` lists them. See [Other people's skills](../guide/other-skills) and [Learn](../guide/learn). |
| `skl open [ref]` | Open a skill, skill set or repo in your browser, or your vault without `ref`. Always prints the URL. See [Web pages](./repos#web-pages). |

## Undo

| Command | Description |
|---|---|
| `skl history` | List generations. |
| `skl rollback [N]` | Go back one generation, or to generation `N`. See [Rollback](../guide/rollback). |

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

## Output

Every command uses the same marks:

| Mark | Means | Example |
|---|---|---|
| `→` | What skl is about to do | `→ copy into someone/pdf` |
| `◇` | A step it did | `◇ saved      pr-review → review/pr-review` |
| `+` `~` `−` | A skill loaded, replaced or updated, unloaded | `+ pdf` |
| `?` `›` | A question; the first answer is the default | `? Add it?   › yes   no` |
| `!` | A warning, with the command that deals with it | `! 2 changes not pushed · skl sync` |
| `✗` | An error, with the command to run | `✗ someone/agent-skills/pdf is someone else's skill` |
| `✓` | The result, last; for a change, its generation | `✓ generation 14 · skl rollback to undo` |

A command that doesn't fit on the line moves to the next one, after `·`.
