# Files

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

## Your vault

Plain folders, so the vault stays readable on GitHub and works without skl, for example as a source for [`npx skills`](../thirdparty/npx-skills#installing-from-your-vault).

```
you/skills/
├── review/
│   ├── pr-review/
│   │   ├── SKILL.md
│   │   └── .skl-origin.toml    # copies and lessons only
│   └── test-gaps/SKILL.md
├── data/
│   └── sql-explain/SKILL.md
├── someone/
│   └── pdf/
│       ├── SKILL.md
│       ├── LICENSE             # their license, kept with the copy
│       └── .skl-origin.toml
├── sets/
│   └── backend.toml
├── lessons/                    # saved interview answers
└── LICENSE                     # your vault's license
```

Every other top-level folder is a group. So `sets` and `lessons` can't be group names.

### `sets/<name>.toml`

A named skill set holds selectors, applied left to right like on the command line. skl writes one selector per line, so when two machines change different selectors, [sync](../guide/sync) usually merges them without a conflict. Edit it by hand; `skl sync` commits the change.

```toml
select = [
  "@base",
  "review",
  "writing/changelog",
  "someone/pdf",
]
```

### `.skl-origin.toml`

```toml
[source]
url = "https://github.com/someone/agent-skills.git//pdf"
commit = "3f9a1c2"
tree = "5dabfb3444c1"   # the author's folder at that commit, as skl landed it
ref = "v1.2"            # only if you pinned one with #
license = "Apache-2.0"

[[lesson]]
peer = "https://github.com/someone/agent-skills.git//deep-pr-review"
commit = "7c21e9f"
prompt = "I like its security checks"
answers = "lessons/7c21e9f.md"
declined = ["inline comments"]
```

`[source]` is written when [`skl learn`](../guide/other-skills#get-one) copies a skill, and only then. A skill Claude built from text has only lessons. A copied skill set keeps the same `[source]` table in its `sets/<name>.toml`.

`tree` is the git tree hash of the author's folder at `commit`, with skl's own changes applied (a new `name`, a license file). If your folder still has that hash, you never changed the skill, so its next update is taken as it is, without Claude. `license` is the license skl found, or `unknown` or `none`, plus your answer if skl asked.

Sources and peers are stored as full URLs, so they mean the same thing on every machine, whatever its `default_host`. After every update, `commit` and `tree` move to the author's version you took or reviewed.

## Loaded skills

Each skill skl loads is a copy, with one extra file:

```
~/.claude/skills/pr-review/
├── SKILL.md
└── .skl-copy.toml
```

```toml
source = "review/pr-review"   # always a skill in your vault
commit = "a91c3e0"            # what it was copied from
hash   = "sha256:9f2c41"      # its files as copied
```

skl uses it to find changes to [save back](../guide/skills#edit-as-usual). A folder without it is a skill skl didn't create.

## Local state

Never synced. Safe to delete, apart from losing your rollback history.

```
~/.skl/
├── config.toml     # this machine's settings, see Configuration
├── vault/          # your vault: a git clone you can edit
├── cache/          # sparse clones skl learn reads, by host and path
├── state.json      # every copy skl made, and where
└── generations/    # one snapshot per change
```
