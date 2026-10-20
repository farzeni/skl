# Commands

Wherever a command takes `<repo>` or `<ref>`, any form from [Repo references](./repos) works.

## Load

| Command | Description |
|---|---|
| `skl use <selectors>` | Replace the loaded set. See [selectors](../guide/loading#selectors). |
| `skl add <selectors>` | Add to the loaded set. |
| `skl drop <selectors>` | Remove from the loaded set. |
| `skl status` | Show what's loaded and where each skill came from. |
| `skl save <name>` | Save the loaded set as a profile. |
| `skl history` | List generations. |
| `skl rollback [N]` | Go back one generation, or to generation `N`. |

## Evolve

| Command | Description |
|---|---|
| `skl fork <ref> [group]` | Copy a skill into your vault and record its source. |
| `skl upstream [skill]` | Show later changes from the sources of your forks and the peers of your lessons. Take them with `--learn ["<text>"]` (you choose, with Claude) or `--merge` (everything). `--all` includes changes you declined before. |
| `skl learn <skill> <peer> ["<text>"]` | Improve your skill with what its peer does better. See [Learn](../guide/learn). |
| `skl star <ref>` | Bookmark a skill without installing it. |

## Vault

| Command | Description |
|---|---|
| `skl init <repo>` | Clone your vault, or set up a new one. |
| `skl sync` | Pull and push your vault. |

## Global flags

| Flag | Effect |
|---|---|
| `--scope user\|project` | Where to link skills. `user` is `~/.claude/skills`; `project` is `./.claude/skills`. Defaults to `scope` in the [config](./config), else `user`. |
| `--dry-run` | Show what would change, and change nothing. |
| `--yes` | Answer yes to confirmations, such as loading a remote skill for the first time. |
