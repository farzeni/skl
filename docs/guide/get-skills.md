# Get skills

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Get a skill from any git repo into your vault, see what it runs before it lands, and keep up with its author.

## Find one

Any git repo with skill folders in it works, on any host. To look skills up by name, the optional [skills.sh part](../thirdparty/skills-sh) adds `skl search`.

## Get one

```sh
skl learn someone/agent-skills/pdf
```

```
→ copy into someone/pdf
  Extract text, tables and forms from PDF files.
  license    Apache-2.0 · notice kept in someone/pdf/LICENSE
  scripts    scripts/extract.py
? Add it?   › yes   no
+ pdf
✓ generation 2 · skl rollback to undo
```

skl fetches only that folder and copies it into your vault as it is, in a group named after the repo's owner. It records where the skill came from in `.skl-origin.toml`, and loads it. No Claude is involved. Before someone else's skill lands, skl shows what it does, its [license](#licenses) and the scripts it runs, and asks once; `--yes` skips the question.

The copy is now one of your skills: [edit it](./skills) as you like. To pin a tag or a commit, or to get a skill from another host, see [Selectors and references](../reference/repos#pinning-a-version).

```sh
skl learn someone/agent-skills/pdf pdf-tools
```

copies it as `pdf-tools` instead. A copy under a new name also gets that name in its `SKILL.md`, so it doesn't clash with the original. If you already have a skill with that name, the same command runs a [lesson](./learn) instead: see [Which skill it changes](./learn#which-skill-it-changes).

## Copy, part or update

| You want | Run | What happens |
|---|---|---|
| The whole skill | `skl learn someone/agent-skills/pdf` | A copy in your vault, no Claude |
| One thing it does well, in a skill you have | `skl learn someone/agent-skills/deep-pr-review review/pr-review "its security checks"` | A [lesson](./learn): Claude adds it to your skill |
| Its author's later changes | `skl learn someone/pdf` | Taken as is if you never changed it, or by a lesson if you did |

## Keep up with its author

`skl sync` checks where each skill you learned came from, and `skl status` shows what it found, without going online:

```
  someone/pdf   update available · skl learn someone/pdf
  data/csv      update available · you changed it · skl learn data/csv
```

To take an update, give `learn` your skill:

```sh
skl learn someone/pdf
```

skl picks how:

```
✓ someone/pdf up to date
→ update someone/pdf (unchanged since copied)
→ lesson into someone/pdf (you changed 2 files)
```

- **Up to date.** The author changed nothing in the skill's folder.
- **Update.** You never changed the skill, so skl replaces it with the author's version. No Claude.
- **Lesson.** You changed it, so Claude merges the author's changes into yours: see [When the peer changes](./learn#when-the-peer-changes). Without the `claude` CLI, skl stops and says so.

`skl learn` with no arguments takes every available update, and `skl learn --dry-run` lists them. An update that changes the author's scripts shows them and asks first, like a new skill.

A skill pinned to a tag or a commit never shows an update. To move it, learn the newer one: `skl learn someone/agent-skills/pdf#v1.3`. A skill that follows a branch updates like any other.

### Drop your changes

To go back to the author's version, say so:

```sh
skl learn someone/pdf "drop my changes, take theirs"
```

This runs a lesson. Claude can restore the author's version as it is, and you see the diff before it lands. `skl rollback` also takes back a change you regret.

## Copy a skill set

`skl learn` also copies a [skill set](./skill-sets) you don't have, with the skills it names:

```sh
skl learn someone/agent-skills/@backend
```

```
→ copy @backend from someone/agent-skills (+ 4 skills into someone/)
```

skl reads their set and the sets it includes, copies each skill it names into the `someone` group, and saves `sets/backend.toml` listing those copies. Add a second argument such as `@their-backend` to save the set under another name.

Their groups all land in one group, so two of their skills can have the same name. The later one is copied under its group's name too, with that name in its `SKILL.md`. A skill already in your vault at that place, which isn't a copy of theirs, is left alone and skipped:

```
~ someone/data-lint   renamed: someone/lint is their review/lint
! someone/deploy skipped: yours, not from someone/agent-skills
  · rename yours, then skl learn someone/agent-skills/@backend
```

To rename a skill of yours, see [Rename or delete a skill](./skills#rename-or-delete-a-skill).

Run the same command again to take their changes. New skills in their set are copied, each skill updates the way a single one does, and skills they removed leave the set file but stay in your vault. Lines you added to the set file yourself stay too. A skill set is never rewritten by Claude.

## Licenses

*This describes what skl does; it isn't legal advice.*

Your vault has its own license. `skl init` writes a `LICENSE` file when it creates a vault: MIT, unless the [`license`](../reference/config) setting says otherwise. An existing vault's `LICENSE` is never touched.

Each skill you learn keeps its author's license. skl looks for it in the skill's folder, then at the root of the author's repo, then in the `license` field of its `SKILL.md`, and prints what it found:

```
  license    Apache-2.0 · notice kept in someone/pdf/LICENSE
```

A license at the root of their repo is copied into the skill's folder, so the notice travels with the copy.

When the license may not fit your vault, skl asks what to do. That's when no license is found, when skl doesn't recognize it, or when it isn't a permissive one (such as MIT, BSD, Apache-2.0 or ISC) and isn't your vault's own:

```
→ copy into someone/report
  license    CC-BY-NC-4.0, non-commercial · your vault is MIT
? Add it?   › yes, keep their license and notice   no   yes, as it is
```

- **Yes, keep their license and notice** copies their license file into the skill's folder.
- **No** lands nothing.
- **Yes, as it is** lands the folder exactly as the author has it.

skl remembers your answer and asks again only if a later update changes the license into one that may not fit. In a lesson, Claude asks the same question in its interview.
