# Roadmap

::: warning Design preview
Parts of skl aren't built yet. This page lists what works today and what comes next.
:::

## What works in v0.1

| Feature | Status | Docs |
|---|---|---|
| `skl init <repo> [selectors]`, with the offer to import the skills already in `~/.claude/skills` | works | [Getting started](../guide/getting-started#make-your-vault) |
| `use`, `add`, `drop` in the user scope (`~/.claude/skills`) | works | [Load, add and drop](../guide/skill-sets#load-add-and-drop) |
| `skl status` and `--json` | works | [What's loaded](../guide/skill-sets#what-s-loaded) |
| Named skill sets (`@name`) and `skl save` | works | [Named skill sets](../guide/skill-sets#named-skill-sets) |
| Edits to loaded skills and hand edits in the vault saved back as generations | works | [Edit as usual](../guide/skills#edit-as-usual) |
| `skl import` | works | [Add a skill you made](../guide/skills#add-a-skill-you-made) |
| `skl history` and `--json`, `skl rollback [N]` | works | [Rollback](../guide/rollback) |
| `skl sync`, with conflicts resolved by hand (`--continue`, `--abort`) | works | [Sync](../guide/sync#by-hand) |
| The `claude` and `agents` targets | works | [Which agents read them](../guide/skill-sets#which-agents-read-them) |
| `skl help`, `--help`, `-h`, `--version` | works | [Commands](../reference/commands) |
| `skl learn`: copies, updates of copies, copied skill sets, license check | v0.2 | [Learn](../guide/learn) |
| `skl learn`: lessons with an engine, sync conflicts resolved by the engine | v0.3 | [Learn](../guide/learn) |
| `skl open`, project scope (`--scope project`), `skl completion` | v0.4 | [Commands](../reference/commands) |
| Third-party parts: `skl search`, the `npx skills` import source | v0.4 | [Third-party parts](../thirdparty/) |

The pages keep their "Design preview" warning until everything on them is built.

## Versions

| Version | Scope |
|---|---|
| **v0.1** | `init`, `sync` (conflicts by hand), `use`, `add`, `drop`, `status`, skill sets, the `claude` and `agents` targets, `import`, `history`, `rollback`, `--help` and `--version`, and `--json` on `status` and `history` |
| **v0.2** | `learn` copies, updates of unchanged copies and copied skill sets (no Claude), license check, update check in `sync` |
| **v0.3** | First a spike on how skl runs an engine for an interview, with `claude` and the opencode and pi templates. Then lessons, including updates of skills you changed, sync conflicts resolved by the engine, and `engine` command templates |
| **v0.4** | `open`, project scope, `skl completion bash\|zsh\|fish`, [third-party parts](../thirdparty/): `skl search`, the `npx skills` import source |
