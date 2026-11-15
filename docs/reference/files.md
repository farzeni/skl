# Files

::: warning Design preview
Parts of this page aren't built yet. See [What works in v0.1](/design/roadmap#what-works-in-v0-1).
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

Every other top-level folder is a group, and every folder in a group that holds a `SKILL.md` is a skill. A `SKILL.md` at any other depth is ignored. `sets`, `lessons` and `h` can't be group names (`-h` always means help). The other rules for names are in [Selectors and references](./repos#grammar).

A name starts with a letter or a digit, so hidden folders are skipped at both levels: `.git` and `.github` aren't groups, and `review/.x/SKILL.md` isn't a skill. A hidden file in `sets/` isn't a skill set either.

skl still counts and lists any other folder or set file whose name is outside the grammar, such as `a.b` or `a b`, but you can't select it by that name:

- **A skill folder** loads with its whole group, or through a valid frontmatter `name`.
- **A group folder** can't be selected at all, so its skills never load.
- **A set file** can't be loaded.

A name that isn't valid UTF-8 is skipped with a warning that says to rename it: `! <path>   skipped: the name isn't valid UTF-8 · rename it in ~/.skl/vault`.

A skill whose `SKILL.md` has no frontmatter, or frontmatter skl can't read, still loads. The command that loads it warns and names the file:

```
! data/pdf/SKILL.md   no frontmatter · edit data/pdf/SKILL.md
```

### `sets/<name>.toml`

A named skill set holds selectors, applied left to right like on the command line. skl writes one selector per line, so when two machines change different selectors, [sync](../guide/sync) usually merges them without a conflict. Edit it by hand; `skl sync` commits the change.

```toml
select = [
  "@base",
  "review",
  "-review/test-gaps",
  "someone/pdf",
]
```

A file skl can't use stops the command before it changes anything, and so does a line that names [someone else's skill](../guide/skill-sets#selectors) or more than one skill. The hint names the file:

```
✗ sets/backend.toml: has no select = [...] · edit sets/backend.toml
```

### `sets/<name>.origin.toml`

A skill set you [copied from someone](../guide/learn#copy-a-skill-set) has a second file next to it, written by skl only:

```toml
[source]
url = "https://github.com/someone/agent-skills//@backend"
commit = "3f9a1c2"
ref = "v1.2"            # only if you pinned one with #
ref_kind = "tag"
selected = [   # the lines skl last wrote into backend.toml
  "someone/lint",
  "someone/deploy",
]
```

`ref` and `ref_kind` mean what they mean in [`.skl-origin.toml`](#skl-origin-toml), and every skill copied with the set records the same pin. When you learn the set again, skl replaces the lines listed in `selected` with their new ones and keeps every line you added. skl never reads a file ending in `.origin.toml` as a skill set.

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

`tree` is the git tree hash of the author's folder at `commit`, with skl's own changes applied (a new `name`, a license file). If your folder still has that hash, you never changed the skill, so its next update is taken as it is, without a lesson.

`ref_kind` says what you pinned: a branch is followed, a tag or a commit [never updates](./repos#pinning-a-version).

`license` is the license skl found, as an SPDX id or expression such as `MIT OR Apache-2.0`, or `unknown` or `none`. When skl [asked about it](../guide/learn#licenses), `license_answer` holds what you chose: `"keep"` (keep their license and notice) or `"as-is"`.

`answers` points to the interview of that lesson, in `lessons/<group>/<skill>/<n>.md`, where `n` counts the skill's lessons from 1.

Commits and trees are stored as full hashes; the examples shorten them. Sources and peers are stored as full URLs, in the [canonical form](./repos#how-a-reference-is-stored), so they mean the same thing on every machine, whatever its `default_host`. After every update, `commit` and `tree` move to the author's version you took or reviewed.

`.skl-origin.toml` stays in your vault: it isn't copied into the loaded skill.

## Loaded skills

Each skill skl loads is a copy in the folder of each [target](./config), with one extra file:

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

skl uses it to find changes to [save back](../guide/skills#edit-as-usual): if the folder's tree hash no longer matches `tree`, it changed. `.skl-copy.toml` itself is never saved back to the vault. With two targets, each copy has its own and is saved back on its own. A folder without it, or one that git tracks in a repo, is a skill skl didn't create. In a repo, loaded skills are untracked files that skl doesn't ignore: see [Keep your skills out of the repo's git](../guide/skill-sets#keep-your-skills-out-of-the-repo-s-git).

## Local state

Never synced.

```
~/.skl/
├── config.toml     # this machine's settings, see Configuration
├── vault/          # your vault: a git clone you can edit
├── cache/          # sparse clones for learn, one per repo
├── state.json      # what's loaded where, update check results
├── generations/    # one snapshot per change
├── staging/        # new copies, before they replace the loaded ones
└── lock            # held while a command changes something
```

`cache/` holds one clone per repo you learn from, at `cache/<host>/<path>` for a remote (`<host>_<port>` with a port, and without a user such as `git@`) and `cache/local/<absolute path>` for a repo on this machine. It's safe to delete: skl fetches what it needs again. `generations/` and `state.json` hold your rollback history. `vault/` may hold commits you haven't pushed: run [`skl sync`](../guide/sync) before you delete it.

If skl can't use `state.json`, it stops instead of overwriting it:

```
✗ can't read ~/.skl/state.json: <reason>
  · move it aside, then run skl status
✗ ~/.skl/state.json was written by a newer skl
  · update skl
```

Moving `state.json` aside forgets the generation history `skl rollback` uses.
