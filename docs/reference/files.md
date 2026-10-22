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
│       ├── LICENSE             # their license, copied
│       └── .skl-origin.toml
├── sets/
│   ├── backend.toml
│   └── backend.origin.toml     # copied sets only
├── lessons/                    # saved interview answers
└── LICENSE                     # your vault's license
```

Every other top-level folder is a group, and every folder in a group that holds a `SKILL.md` is a skill. A `SKILL.md` at any other depth is ignored. The rules for group names are in [Selectors and references](./repos#grammar).

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

### `sets/<name>.origin.toml`

A skill set you [copied from someone](../guide/learn#copy-a-skill-set) has a second file next to it, written by skl only:

```toml
[source]
url = "https://github.com/someone/agent-skills//@backend"
commit = "3f9a1c2"
selected = [   # the lines skl last wrote into backend.toml
  "someone/lint",
  "someone/deploy",
]
```

When you learn the set again, skl replaces the lines listed in `selected` with their new ones and keeps every line you added. skl never reads a file ending in `.origin.toml` as a skill set.

### `.skl-origin.toml`

```toml
[source]
url = "https://github.com/someone/agent-skills//skills/pdf"
commit = "3f9a1c2"
tree = "5dabfb3444c1"   # the author's folder, as skl landed it
ref = "v1.2"            # only if you pinned one with #
ref_kind = "tag"        # "branch", "tag" or "commit"
license = "Apache-2.0"

[[lesson]]
peer = "https://github.com/someone/agent-skills//deep-pr-review"
commit = "7c21e9f"
prompt = "I like its security checks"
answers = "lessons/review/pr-review/1.md"
declined = ["inline comments"]
```

`[source]` is written when [`skl learn`](../guide/learn#get-a-skill) copies a skill, and only then. A skill Claude built from text has only lessons.

`tree` is the git tree hash of the author's folder at `commit`, with skl's own changes applied (a new `name`, a license file). If your folder still has that hash, you never changed the skill, so its next update is taken as it is, without Claude.

`ref_kind` says what you pinned: a branch is followed, a tag or a commit [never updates](./repos#pinning-a-version).

`license` is the license skl found, or `unknown` or `none`. When skl [asked about it](../guide/learn#licenses), `license_answer` holds what you chose: `"keep"` (keep their license and notice) or `"as-is"`.

`answers` points to the interview of that lesson, in `lessons/<group>/<skill>/<n>.md`, where `n` counts the skill's lessons from 1.

Commits and trees are stored as full hashes; the examples shorten them. Sources and peers are stored as full URLs, in the [canonical form](./repos#how-a-reference-is-stored), so they mean the same thing on every machine, whatever its `default_host`. After every update, `commit` and `tree` move to the author's version you took or reviewed.

`.skl-origin.toml` stays in your vault: it isn't copied into the loaded skill.

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
tree   = "9f2c41d07e3b"       # git tree hash of the folder
```

skl uses it to find changes to [save back](../guide/skills#edit-as-usual): if the folder's tree hash no longer matches `tree`, it changed. `.skl-copy.toml` itself is never saved back to the vault. A folder without it is a skill skl didn't create.

## Local state

Never synced.

```
~/.skl/
├── config.toml     # this machine's settings, see Configuration
├── vault/          # your vault: a git clone you can edit
├── cache/          # sparse clones for learn, by host and path
├── state.json      # what's loaded where, update check results
├── generations/    # one snapshot per change
└── lock            # held while a command changes something
```

`cache/` is safe to delete: skl fetches what it needs again. `generations/` and `state.json` hold your rollback history. `vault/` may hold commits you haven't pushed: run [`skl sync`](../guide/sync) before you delete it.
