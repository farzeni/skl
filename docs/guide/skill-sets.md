# Skill sets

::: warning Design preview
Parts of this page aren't built yet. See [What works in v0.1](/design/roadmap#what-works-in-v0-1).
:::

Load only the skills a task needs, and switch to a saved mix with one word on any machine.

A skill set is a list of selectors: the groups and skills you load together. The one you have loaded now is your **active skill set**. Save it with a name and it becomes a **named skill set**, which you load anywhere with `@name`.

## Why load fewer

Your agent sees the description of every loaded skill on each turn. Claude Code keeps that list within 1% of the context window. Past it, Claude Code drops the descriptions of the skills you use least, and Claude is less likely to pick those on its own ([Claude Code docs](https://code.claude.com/docs/en/skills#skill-descriptions-are-cut-short)). Codex caps it at 2%. Others, such as pi, list every skill, so each one costs context on every turn. Loading only what a task needs keeps every description in view. Nothing is deleted: what you don't load stays in your vault.

## Load, add and drop

```sh
skl use @backend writing/changelog -someone/pdf
```

| Command | Effect |
|---|---|
| `skl use <selectors>` | Replace the active skill set |
| `skl add <selectors>` | Add to it |
| `skl drop <selectors>` | Remove from it |
| `skl status` | Show what's loaded, where it came from, your named skill sets, and what needs you |

`use`, `add` and `drop` first say what they're about to do, with `load`, `add` or `drop` and the selectors you typed, then what changed:

```
→ add review/test-gaps
+ test-gaps
✓ generation 4 · skl rollback to undo
```

They also bring your loaded skills in line with your vault. A skill that changed there is copied again, and so is a loaded folder you deleted by hand, so use `skl drop` to unload a skill. Both show the same line:

```
~ pr-review   updated
```

Add `--dry-run` to any of them to see the changes without applying them: see [Dry run](../reference/commands#dry-run).

Loaded skills are plain folders. Changes you make to them are saved back to your vault: see [Your skills](./skills).

## What's loaded

`skl status` shows each scope you have skills in, what's loaded there and from which vault skill, then everything that needs you, each with the command to run:

```
loaded (user)  @backend writing/changelog
  pr-review     review/pr-review    changed · skl sync
  sql-explain   data/sql-explain
  changelog     writing/changelog
  pdf           someone/pdf         update available · skl learn someone/pdf
! review/old-name   matches no skill (in @backend) · edit sets/backend.toml
! lint   skipped: ~/.claude/skills/lint is not in your vault
  · move ~/.claude/skills/lint aside, then skl sync
loaded (project ~/code/prj1)  @frontend
  ui-review     frontend/ui-review  vault changed · skl sync
not in your vault
  my-new-skill  · skl import my-new-skill
  db-notes      · skl import db-notes --scope project
skill sets      @backend  @base  @frontend
vault           2 changes not pushed · skl sync
```

The project scope shows only inside a git repo that has skills loaded: see [For one repo only](#for-one-repo-only). `skl status` never goes online: `update available` is what the last [`skl sync`](./sync) found. For scripts, add `--json`: see [JSON](../reference/commands#json).

## Named skill sets

```sh
skl use @base review -review/test-gaps someone/pdf
skl save backend
```

saves the selectors of your active skill set as `sets/backend.toml` in your vault, one selector per line. That's your user scope's active skill set; with `--scope project`, the [repo's](#for-one-repo-only).

```toml
select = [
  "@base",
  "review",
  "-review/test-gaps",
  "someone/pdf",
]
```

Later `add` and `drop` commands change your active skill set, not the file: run `skl save backend` again to keep them. If your active skill set includes `@backend` itself, skl writes the lines of `@backend` in its place, so the file keeps its old lines plus your changes. Because a skill set keeps selectors, not a list of skills, a skill you add to the `review` group later is part of `@backend` too. A skill set can include other skill sets.

If `backend` already exists, skl shows how its selectors change and asks before overwriting it. If the new skill set is empty, so every line would go, the default answer is no:

```
→ overwrite sets/backend.toml
− @base
− review
? Overwrite @backend?   › no   yes
```

Answer yes, or pass `--yes`, to save the empty skill set.

To change a named skill set, edit its file in `~/.skl/vault/sets/`. To delete one, delete the file. `skl status` lists your named skill sets. Run `skl sync` to commit these edits and take them to your other machines: see [Sync](./sync).

## Selectors

Selectors are applied left to right, and on a [name clash](#names-and-clashes) the later one wins.

| Selector | Loads |
|---|---|
| `review` | Every skill in one of your groups |
| `review/pr-review` | One skill from one of your groups |
| `@backend` | A named skill set from your vault |
| `-review/test-gaps` | Removes a skill that an earlier selector added |

Every selector names something in your vault. To use a skill or a skill set from someone else's repo, [learn it](./learn#get-a-skill) first: it becomes a skill in your vault. Given a reference to someone else's repo, `use`, `add` and `drop` stop and say so:

```
✗ someone/agent-skills/pdf is someone else's skill
  · skl learn someone/agent-skills/pdf
```

A line in a named skill set that names someone else's repo stops too, loads nothing and [exits with `1`](../reference/commands#exit-codes). Learning the skill wouldn't change the line, so skl points at the file:

```
✗ @backend: someone/agent-skills/pdf is someone else's skill
  · edit sets/backend.toml
```

## For one repo only

Load a skill set for one repo only, next to the one you load everywhere. Say you want `@reviews` in every repo, `@frontend` only in `prj1` and `@backend` only in `prj2`:

```sh
skl use @reviews
cd ~/code/prj1 && skl use @frontend --scope project
cd ~/code/prj2 && skl use @backend --scope project
```

```
→ load @backend in ~/code/prj2
+ sql-explain   + api-design
✓ generation 3 · skl rollback to undo
```

`--scope project` copies the skills into `.claude/skills` at the root of the git repo you're in, or `.agents/skills` for the [`agents` target](#which-agents-read-them). Outside a git repo, it's an error. A Claude Code session that's already running may need `/reload-skills` after the repo's first load: see [Which agents read them](#which-agents-read-them).

Each repo has its own active skill set, and your user scope has one too. In `prj1`, Claude Code sees `@reviews` and `@frontend`; in `prj2`, `@reviews` and `@backend`; anywhere else, only `@reviews`. If both scopes load a skill with the same name, the user one wins: see [Names and clashes](#names-and-clashes).

### Work in a repo

Inside a repo, add `--scope project` to act on that repo's active skill set. Without it, a command acts on your user scope, even inside a repo.

| Command | With `--scope project` |
|---|---|
| `skl use`, `add`, `drop` | Change the repo's active skill set. |
| `skl save <name>` | Save the repo's selectors as `@name`. |
| `skl learn <peer>` | A new skill joins the repo's active skill set. |
| `skl import <name>` | Import a skill from the repo's `.claude/skills`. `skl status` shows the command for each one. |
| `skl init <repo>` | Also offer to import the repo's skills that aren't in a vault. |

So to keep a skill you learned in `prj1` as part of `@frontend`:

```sh
skl learn someone/agent-skills/storybook --scope project
skl save frontend --scope project
```

Edits to a loaded skill are [saved back](./skills#edit-as-usual) at the start of the next command that changes something, whichever repo you run it in. Loading changes only your user scope and the repo you're in. So after your vault changes elsewhere, for example with a `skl sync` in another folder, a repo's skills can be behind it: `skl status` in that repo marks them `vault changed`, and `skl sync` there reloads them. A [rollback](./rollback) undoes its generations in every repo they changed.

A git worktree is a repo of its own: load its skill set there. In a worktree without a `.claude/skills` folder, Claude Code shows the main checkout's project skills.

### On another machine

A repo's active skill set is kept on this machine, in `~/.skl/state.json`, not in your vault. Save it as a named skill set, as above, and load it once in each repo on your next machine:

```sh
skl init you/skills @reviews
cd ~/code/prj1 && skl use @frontend --scope project
cd ~/code/prj2 && skl use @backend --scope project
```

### Skills the repo commits

Claude Code's docs suggest committing a repo's `.claude/skills` so the whole team gets them. A skill folder git tracks belongs to the repo: skl never changes it, and `skl status` doesn't list it. If a skill you load has its name, skl skips yours and warns:

```
! team-style   skipped: .claude/skills/team-style is committed in this repo
  · skl drop frontend/team-style --scope project
```

To have a committed skill in your vault, learn it from the repo, so the team's later changes reach you as [updates](./learn#keep-up-with-its-author). `skl import` says so:

```
✗ team-style is committed in this repo · skl learn team/prj1/team-style
```

### Keep your skills out of the repo's git

skl doesn't touch the repo's git. The skills it loads into a repo are untracked files: `git status` lists them, and `git add -A` would commit them, `.skl-copy.toml` included, and hand your skills to the team. To keep them out, list each folder in `.git/info/exclude`, for you only, or in the repo's `.gitignore`, for everyone:

```
/.claude/skills/ui-review/
```

Claude Code's docs don't say whether it loads a project skill that git ignores; that's not tested yet.

## Which agents read them

skl loads plain skill folders in the [Agent Skills](https://agentskills.io/specification) format, which other agents read too. It writes them into the folders of your **targets**:

| Target | User scope | Project scope |
|---|---|---|
| `claude`, the default | `~/.claude/skills` | `.claude/skills` |
| `agents` | `~/.agents/skills` | `.agents/skills` |

To load into both, set `targets` in your [config](../reference/config):

```toml
targets = ["claude", "agents"]
```

Each target then holds its own copy of every loaded skill, and changes to either copy are [saved back](./skills#edit-as-usual) to your vault.

Take a target out of `targets` and the next load unloads its copies, naming the folder:

```
− pr-review   from ~/.agents/skills
```

skl never removes a copy with changes that aren't saved back.

| Agent | Reads `claude` | Reads `agents` | After `skl use` |
|---|---|---|---|
| Claude Code | yes | not in its docs | sees the change, except a new skills folder, below |
| opencode | yes | yes, first | restart it |
| pi | with one setting, below | yes | `/reload` |
| Codex | no | yes | sees the change, else restart it |
| Gemini CLI | no | yes | `/skills reload` |
| GitHub Copilot | project scope only | yes | not stated |
| Cursor | yes | yes | restart it |
| Amp | yes | yes | `reload_skills` |

From each agent's docs, October 2026; opencode and pi were tested. Claude Code watches its skills folders, but not one created after the session started: after a repo's first `--scope project` load, if the repo had no `.claude/skills` when you started Claude Code, run `/reload-skills` in it. pi reads `~/.claude/skills` too if `~/.pi/agent/settings.json` holds `{"skills": ["~/.claude/skills"]}`. Use that or the `agents` target, not both: pi warns about every skill it finds twice.

Pick the one target your agents read. Load into both only when one of your agents reads just `~/.claude/skills` and another just `~/.agents/skills`, such as Claude Code and Codex. An agent that reads both folders then sees each skill twice and keeps one.

skl copies a skill as it is. Frontmatter fields that only Claude Code knows, such as `context: fork` or `hooks`, do nothing in other agents, which still load the skill.

## Names and clashes

Claude Code reads skills from one flat folder, so skl copies each skill under its folder name: `review/pr-review` becomes `~/.claude/skills/pr-review`, and `~/.agents/skills/pr-review` for the `agents` target. A skill's names are its folder name and the `name` in its `SKILL.md` frontmatter. Two skills clash when they share a name, so `a/x` and `b/y` with `name: x` clash.

Names that differ only by case or Unicode form are the same name, because a disk can store them as one folder, as the macOS default does. `low/PDF2` and `Up/pdf2` clash, and so do `é` written as one character and `e` plus an accent. Group names clash the same way: `Review` and `review` make a selector ambiguous, so skl stops and lists both, as it does for a name in two groups.

A selector that names one skill in a group, or one skill set, finds it like this:

- **Two skills that differ only by case or form:** an exact folder name selects that one. With `data/PDF` and `data/pdf`, `data/pdf` selects the second, and `data/Pdf` stops and lists both.
- **Two skills that share a frontmatter `name`:** a selector naming it stops and lists both. With `name: pdf` in `data/a` and `data/b`, `data/pdf` stops; `data/a` selects that folder.
- **A folder name and a frontmatter name:** the folder name wins. `data/PDF` picks the folder `data/pdf` over `data/reader` with `name: PDF`.
- **Skill sets** match in any case or form, so `@BASE` loads `base.toml`. An exact name selects that file. With `Dup.toml` and `dup.toml`, `@dup` selects the second, and `@DUP` stops and lists both.

Loading a whole group stops if two skills in it clash, such as `data/PDF` and `data/pdf`, or two skills with the same frontmatter `name`. This goes for `skl use data` and for a line `data` in a named skill set. skl lists both, loads nothing and [exits with `1`](../reference/commands#exit-codes):

```
✗ data/PDF and data/pdf have the same name · rename one in your vault
```

Removals apply first, so `skl use data -data/PDF` loads `data/pdf` and doesn't stop.

Between the skills you select, the later selector wins, and skl says which skill replaced which:

```sh
skl use review someone/pr-review
```

```
~ pr-review   someone/pr-review replaces review/pr-review
```

Removals apply before clashes are settled, so a skill you removed replaces nothing. If `@backend` holds `review` and `someone/pr-review`, this keeps `review/pr-review` loaded, with no `~` line:

```sh
skl use @backend -someone/pr-review
```

A skill loses a clash only to a later skill that stays loaded, so every `~` line is a real replacement.

A skill that lost a clash is still selected, so you can drop it. After `skl use review someone/pr-review`, `skl drop review/pr-review` adds the line `-review/pr-review` to your active skill set and changes no folder. Dropping `someone/pr-review` later then unloads `pr-review` instead of bringing `review/pr-review` back.

Claude Code prefers a user skill to a project skill with the same name. When a skill you loaded with `--scope project` is hidden that way, `skl status` warns about it.

## Skills skl didn't create

Skills you added by hand, installed with another tool such as [`npx skills`](../thirdparty/npx-skills), or installed through Claude Code plugins show up in `skl status` as not in your vault, from the folders of every target. skl never changes or removes them. Skills a repo commits aren't listed: see [Skills the repo commits](#skills-the-repo-commits). Plugin skills never clash, because Claude Code prefixes their names with the plugin's.

If a hand-made skill already has a name that a loaded skill needs, skl skips its own skill, loads the rest, and warns you. The command still succeeds:

```
! pr-review   skipped: ~/.claude/skills/pr-review is not in your vault
  · move ~/.claude/skills/pr-review aside, then skl sync
```

`skl status` keeps listing it as skipped until the name is free. Your vault already has the name, so [`skl import`](./skills#add-a-skill-you-made) refuses the hand-made folder. Move it out of `~/.claude/skills`, and the next load puts your vault's skill there: `skl sync` reloads what's loaded.

opencode reads `~/.agents/skills` before `~/.claude/skills`. So even without the `agents` target, `skl status` warns when a skill there that skl didn't create hides one you loaded:

```
! pdf   hidden in opencode by ~/.agents/skills/pdf
  · rename or remove one of them
```

`skl save` saves your selectors as they are; the skipped skills load once their names are free. To keep one in your vault, import it: see [Add a skill you made](./skills#add-a-skill-you-made).
