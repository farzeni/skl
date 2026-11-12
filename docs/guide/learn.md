# Learn

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Get a skill from any git repo, or teach a skill you have what another one does better, and keep up with its author. It's all one command, `skl learn`, and skl picks what to do.

| You want | Run | What happens |
|---|---|---|
| A skill you don't have | `skl learn someone/agent-skills/pdf` | A copy in your vault, no Claude |
| One thing it does well, in a skill you have | `skl learn someone/agent-skills/deep-pr-review review/pr-review "its security checks"` | A [lesson](#teach-your-skill-a-trick): Claude adds it to your skill |
| Only part of it, as a new skill | `skl learn someone/agent-skills/docx "only the tables part"` | A lesson into an [empty skill](#a-new-skill-from-part-of-another) |
| Its author's later changes | `skl learn someone/pdf` | Taken as is if you never changed it, or by a lesson if you did |

## Find a skill

Any git repo with skill folders in it works, on any host. To look skills up by name, the optional [skills.sh part](../thirdparty/skills-sh) adds `skl search`.

## Get a skill

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

skl fetches only that folder and copies it into your vault as it is, in a group named after the repo's owner. It records where the skill came from in `.skl-origin.toml`, and loads it: the skill joins your active skill set, as `skl add someone/pdf` would. No Claude is involved. Before someone else's skill lands, skl shows what it does, its [license](#licenses) and the scripts it runs, and asks once; `--yes` skips the question (see [Questions](../reference/commands#questions)). Scripts are every file in the skill that isn't Markdown (`*.md`).

The copy is now one of your skills: [edit it](./skills) as you like. To pin a tag or a commit, or to get a skill from another host, see [Selectors and references](../reference/repos#pinning-a-version).

```sh
skl learn someone/agent-skills/pdf pdf-tools
```

copies it as `pdf-tools` instead. A copy under a new name also gets that name in its `SKILL.md`, so it doesn't clash with the original. If you already have a skill with that name, the same command runs a lesson instead: see [Which skill it changes](#which-skill-it-changes).

## Teach your skill a trick

When you already have the skill, `learn` improves yours with what the other one does better. Claude reads both, asks you what to take and how, and rewrites yours. You review the diff before anything lands. The skill you learn from is its **peer**.

```sh
skl learn someone/agent-skills/deep-pr-review review/pr-review \
  "I like its security checks"
```

The first argument is the peer, given as any [repo reference](../reference/repos) or as one of your own skills. The second is your skill. Leave it out to use your skill with the peer's name. The text at the end is optional and goes to Claude as is, so say what you want in your own words. Without it, Claude asks.

### How a lesson works

1. **Analyse.** Claude maps what both skills share, what only the peer does, what only yours does, and where they contradict each other.
2. **Interview.** One question for each decision: what you asked for, and every contradiction. Your answers are saved.
3. **Rewrite.** Claude rewrites your skill in its own voice and structure, adding the new capability. The peer's scripts come along only if you said yes.
4. **Review.** skl shows the diff.
5. **Land.** You replace your skill, ask for changes in plain words ("make the security part shorter") and review again, save the result under a new name, or discard it. A discarded lesson changes nothing and records nothing.

The examples show what Claude asks. The questions appear in Claude Code's own interface, or your [engine](#who-does-the-thinking)'s, so their exact look may differ. A lesson needs a terminal, since you answer its questions.

```
→ lesson into review/pr-review
◇ peer only  OWASP checklist, secret scanning ← what you asked for
◇ clash      peer comments inline; yours writes one summary
? Inline comments or one summary?
  › keep my summary, add a Security section
? Bring along the peer's secret-scanning script?
  › yes, as an optional step
◇ rewritten  SKILL.md +38 −5
? Land it
  › replace   change it   save as review/pr-review-sec   discard
✓ generation 12 · skl rollback to undo
```

Landing creates a vault commit and a generation, so `skl rollback` undoes it. Run `skl sync` to push it.

To learn from more than one peer, run `skl learn` once for each. Every run is its own lesson, and you can roll each one back separately.

## A new skill from part of another

With text and a skill you don't have, Claude builds a new skill from the peer with only what you asked for. It runs like a [lesson](#how-a-lesson-works), starting from an empty skill: Claude asks what to bring along with that part, and which scripts to take.

```sh
skl learn someone/agent-skills/docx "only the tables part"
```

## Keep up with its author

`skl sync` checks where each skill you learned came from, and `skl status` shows what it found, without going online:

```
  someone/pdf   update available · skl learn someone/pdf
  data/csv      update available · you changed it · skl learn data/csv
```

To take an update, give `learn` only your skill:

```sh
skl learn someone/pdf
```

skl uses the skill it was copied from, or the peer you learned from last, and picks how:

```
✓ someone/pdf up to date
→ update someone/pdf (unchanged since copied)
→ lesson into someone/pdf (you changed 2 files)
```

- **Up to date.** The author changed nothing in the skill's folder.
- **Update.** You never changed the skill, so skl replaces it with the author's version. No Claude.
- **Lesson.** You changed it, so Claude merges the author's changes into yours. Claude sees three versions: the author's as you last took it, the author's now, and yours, plus skl's own attempt at merging them. So it can tell their changes from yours, and asks only about what's new and what contradicts. It reuses your saved answers, and changes you skip aren't asked about again, unless your text asks for them. Without the [engine](#who-does-the-thinking)'s CLI, skl stops and says so.

Text narrows it down: `skl learn review/pr-review "only the new security checks"`. With text, skl always runs a lesson, even if the author changed nothing.

`skl learn` with no arguments takes every available update, and `skl learn --dry-run` lists them. Updates that need a lesson run one at a time; discarding one moves on to the next. An update that changes the author's scripts, meaning any file that isn't Markdown, or its mode, shows them and asks first, like a new skill.

A skill pinned to a tag or a commit never shows an update. To move it, learn the newer one: `skl learn someone/agent-skills/pdf#v1.3`. A skill that follows a branch updates like any other.

### Drop your changes

To go back to the author's version, say so:

```sh
skl learn someone/pdf "drop my changes, take theirs"
```

This runs a lesson. Claude can restore the author's version as it is, and you see the diff before it lands. `skl rollback` also takes back a change you regret.

## Which skill it changes

| Second argument | Your skill |
|---|---|
| none | the one with the peer's name: `pdf` |
| `pdf-tools` | the one with that name, in whichever group has it |
| `docs/pdf` | exactly that one |
| `docs/` | the one with the peer's name in your `docs` group |

A name matches a skill's folder or the `name` in its `SKILL.md`. If two groups have it, skl stops and asks you to write `group/name`. If no skill has it, a new one goes into a group named after the peer's owner: `someone/agent-skills/pdf` becomes `someone/pdf`, and a repo on this machine uses its folder name. A name that can't be a group follows the [reserved names](../reference/repos#grammar) rule. A skill at the root of a repo (`someone/pdf-skill//`) takes the `name` in its `SKILL.md`, else the repo's name.

A peer that's one of your own skills is never copied: your vault already has it. To start a new skill from it, say what to take, and a lesson builds it:

```
✗ review/pr-review is already in your vault
  · skl learn review/pr-review pr-review-sec "<what to take>"
```

The last argument counts as text when it contains whitespace, such as a space. So a one-word request needs more words, or it's read as a skill name.

skl prints what it's about to do before it does it:

```
→ copy into someone/pdf
→ lesson into review/pr-review
→ new skill someone/docx, learned from someone/agent-skills/docx
```

It asks only before someone else's new skill or scripts land, and when a lesson is ready to land. If it isn't what you meant, `skl rollback` undoes it.

## Copy a skill set

`skl learn` also copies a [skill set](./skill-sets) you don't have, with the skills it names:

```sh
skl learn someone/agent-skills/@backend
```

```
→ copy @backend from someone/agent-skills
  4 skills into someone/
```

skl reads their set and the sets it includes, copies each skill it names into the `someone` group, and saves `sets/backend.toml` listing those copies. Where the set came from is kept next to it, in [`sets/backend.origin.toml`](../reference/files#sets-name-origin-toml). Add a second argument such as `@their-backend` to save the set under another name.

Their groups all land in one group, so two of their skills can have the same name. The later one is copied under its group's name too, with that name in its `SKILL.md`. A skill already in your vault at that place, which isn't a copy of theirs, is left alone and skipped:

```
◇ renamed    someone/data-lint: someone/lint is their review/lint
! someone/deploy skipped: yours, not from someone/agent-skills
  · rename yours, then skl learn someone/agent-skills/@backend
```

To rename a skill of yours, see [Rename or delete a skill](./skills#rename-or-delete-a-skill).

Run the same command again to take their changes. New skills in their set are copied, each skill updates the way a single one does, and skills they removed leave the set file but stay in your vault. Lines you added to the set file yourself stay too. A skill set is never rewritten by Claude.

## Who does the thinking

For a lesson, skl prepares a workspace with both skills and runs the **engine** in it: the `claude` CLI by default, using your existing login. Claude writes its result to a staging folder. Only skl commits to your vault, and only after you choose. A copy, and an update to a copy you never changed, need no engine. Building a new skill from text, and updating a skill you changed, do: without the engine's CLI, skl stops and says so.

Each run with the engine is recorded in the skill's `.skl-origin.toml` as a lesson. It holds the peer, its commit, what you asked for, your answers and what you declined. A copy records its source instead. See [Files](../reference/files#skl-origin-toml).

### Other agents

To run lessons and sync conflicts in another agent, set [`engine`](../reference/config) to the command line skl runs in the lesson workspace:

```toml
engine = "opencode --prompt {prompt}"     # opencode
engine = "pi --skill {skill} {prompt}"    # pi
```

| Placeholder | Becomes |
|---|---|
| `{prompt}` | The first prompt: it names the lesson skill and the brief. |
| `{skill}` | The path of the bundled lesson or merge skill in the workspace, for agents that load a skill by path. |

skl splits the line into words at spaces, keeping text in double quotes together, and runs the first word from your `PATH`. There's no shell, so `$VAR`, `~` and pipes stay as they are. Each placeholder becomes exactly one argument, whatever it holds, also inside a word such as `--skill={skill}`. The agent uses its own login, and finds skl's lesson skill in the workspace's `.claude/skills` or `.agents/skills` if it doesn't load it by path. What every engine must do is in [Learn engine](../design/architecture#learn-engine).

A line skl can't use stops the lesson before anything starts:

```
✗ engine: unknown placeholder {promt} · edit engine in ~/.skl/config.toml
```

The same goes for an unclosed quote or a command that isn't on your `PATH`. The examples above aren't tested yet: they are checked before lessons ship in v0.3, see [Roadmap](../design/roadmap).

## Licenses

*This describes what skl does; it isn't legal advice.*

Your vault has its own license. `skl init` writes a `LICENSE` file when it creates a vault: MIT, unless the [`license`](../reference/config) setting says otherwise. An existing vault's `LICENSE` is never touched. To know your vault's license, skl reads its `LICENSE` file, the same way it reads an author's, and uses the `license` setting only when there's no file it recognizes.

For MIT and Apache-2.0, skl writes the full text; MIT's copyright line has the year and your git name. For any other SPDX id, it writes one line, `SPDX-License-Identifier: <id>`, and you paste the full text yourself. With `none`, there's no `LICENSE` file, and the new vault still starts with generation 1.

Each skill you learn keeps its author's license. skl looks for a `LICENSE*` or `COPYING*` file in the skill's folder, then at the root of the author's repo, and prints what it found:

```
  license    Apache-2.0 · notice kept in someone/pdf/LICENSE
```

A license at the root of their repo is copied into the skill's folder, so the notice travels with the copy.

If the first place with a license file has several, such as `LICENSE-MIT` and `LICENSE-APACHE`, skl reads them all and records the choice as one SPDX expression, `MIT OR Apache-2.0`. A root license is copied with all its files.

Only when neither place has a license file does skl read the `license` field in `SKILL.md`, and only if it's an SPDX id or expression, such as `MIT`. Any other text, such as `Complete terms in LICENSE.txt`, is ignored: a file it points to is found in the folder first, and without one the license is unknown. A license found only there comes with no file to copy, and skl says so:

```
  license    MIT (from SKILL.md, no license file)
```

When the license may not fit your vault, skl asks what to do. That's when no license is found, when skl doesn't recognize it, or when it isn't permissive and isn't your vault's own. These are permissive: MIT, MIT-0, 0BSD, BSD-2-Clause, BSD-3-Clause, Apache-2.0, ISC, Zlib, BSL-1.0, Unlicense and CC0-1.0. With a choice such as `MIT OR GPL-3.0`, skl asks only if no choice is permissive or your vault's own. A vault with no license (`license = none` and no `LICENSE` file) has no license of its own to match, so every license that isn't permissive makes skl ask:

```
→ copy into someone/report
  license    CC-BY-NC-4.0, non-commercial · your vault is MIT
? Add it?
  › yes, keep their license and notice   no   yes, as it is
```

- **Yes, keep their license and notice** copies their license file into the skill's folder.
- **No** lands nothing.
- **Yes, as it is** lands the folder exactly as the author has it.

skl remembers your answer and asks again only if a later update changes the license into one that may not fit. In a lesson, Claude asks the same question in its interview.
