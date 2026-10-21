# Files

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

## Your vault

Plain folders, so the vault stays readable on GitHub and works without skl: as a Claude Code plugin marketplace, and as a source for `npx skills`.

```
you/skills/
├── review/
│   ├── pr-review/
│   │   ├── SKILL.md
│   │   └── .skl-origin.toml    # only for forks and skills with lessons
│   └── test-gaps/SKILL.md
├── data/
│   └── sql-explain/SKILL.md
├── profiles/
│   └── backend.toml
├── lessons/                    # saved interview answers
└── stars.toml
```

### `profiles/<name>.toml`

A profile holds selectors, applied left to right like on the command line. Edit it by hand; `skl sync` commits the change.

```toml
select = ["@base", "review", "writing/changelog", "anthropics/skills/pdf"]
```

### `.skl-origin.toml`

```toml
[fork]
source = "https://github.com/someone/agent-skills.git//pdf"
commit = "3f9a1c2"

[[lesson]]
peer     = "https://github.com/someone/agent-skills.git//deep-pr-review"
commit   = "7c21e9f"
prompt   = "I like its security checks"
answers  = "lessons/7c21e9f.md"
declined = ["inline comments"]
```

Sources and peers are stored as full URLs, so they mean the same thing on every machine, whatever its `default_host`. After `skl upstream --merge` or `skl learn`, `commit` moves to the upstream commit you reviewed.

### `stars.toml`

```toml
[[star]]
ref  = "someone/agent-skills/deep-pr-review"
note = "good security checks"
```

## Local state

Never synced. Safe to delete, apart from losing your rollback history.

```
~/.skl/
├── config.toml     # this machine's settings, see Configuration
├── vault/          # clone of your vault: a plain git repo you can edit
├── cache/          # sparse clones of other repos, by host and path
├── state.json      # every link skl created
└── generations/    # one snapshot per change
```
