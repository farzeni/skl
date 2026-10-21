# Commands

Wherever a command takes `<repo>` or `<ref>`, any form from [Repo references](./repos) works.

## Load

| Command | Description |
|---|---|
| `skl use <selectors>` | Replace the loaded set. See [selectors](../guide/loading#selectors). |
| `skl add <selectors>` | Add to the loaded set. |
| `skl drop <selectors>` | Remove from the loaded set. |
| `skl status` | Show what's loaded, where each skill came from, and your profiles. |
| `skl save <name>` | Save the selectors behind the loaded set as a profile. Asks before overwriting one. See [Profiles](../guide/loading#profiles). |
| `skl import <name> [group]` | Copy a skill skl didn't create into your vault. See [Skills skl didn't create](../guide/loading#skills-skl-didn-t-create). |
| `skl history` | List generations. |
| `skl rollback [N]` | Go back one generation, or to generation `N`. |

## Evolve

| Command | Description |
|---|---|
| `skl fork <ref> [group]` | Copy a skill into your vault and record its source. |
| `skl upstream [skill]` | Show later changes from the sources of your forks and the peers of your lessons. `--merge` takes everything from a fork's source; see [Fork](../guide/fork). |
| `skl learn <skill> [peer] ["<text>"]` | Improve your skill with what its peer does better. Without a peer, learn again from the last one, or a fork's source. See [Learn](../guide/learn). |
| `skl star <ref>` | Bookmark a skill without installing it. |

## Vault

| Command | Description |
|---|---|
| `skl init <repo>` | Clone your vault, or set up a new one. |
| `skl sync` | Commit hand edits, pull, push, and reload what's loaded. On a conflict, resolve with Claude, by hand or abort; `--continue` and `--abort` finish or undo a merge. See [Sync](../guide/sync). |

## Global flags

| Flag | Effect |
|---|---|
| `--scope user\|project` | Where to link skills. `user` is `~/.claude/skills`; `project` is `./.claude/skills`. Defaults to `scope` in the [config](./config), else `user`. |
| `--dry-run` | Show what would change, and change nothing. |
| `--yes` | Answer yes to confirmations, such as loading a remote skill for the first time. |
