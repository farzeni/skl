# Skill sets

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Load only the skills a task needs, and switch to a saved mix with one word on any machine.

A skill set is a list of selectors: the groups and skills you load together. The one you have loaded now is your **active skill set**. Save it with a name and it becomes a **named skill set**, which you load anywhere with `@name`.

## Why load fewer

Claude Code lists every skill in `~/.claude/skills` to Claude on each turn, within a budget of 1% of the context window. Past it, Claude Code drops the descriptions of the skills you use least, and Claude is less likely to pick those on its own ([Claude Code docs](https://code.claude.com/docs/en/skills#skill-descriptions-are-cut-short)). Loading only what a task needs keeps every description in view. Nothing is deleted: what you don't load stays in your vault.

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

Add `--dry-run` to any of them to see the changes without applying them: see [Dry run](../reference/commands#dry-run).

Loaded skills are plain folders. Changes you make to them are saved back to your vault: see [Your skills](./skills).

## What's loaded

`skl status` shows each scope you have skills in, what's loaded there and from which vault skill, then everything that needs you, each with the command to run:

```
loaded (user)  @backend writing/changelog
  pr-review     review/pr-review    changed · skl sync
  sql-explain   data/sql-explain
  changelog     writing/changelog
  pdf           someone/pdf         update available
                · skl learn someone/pdf
! review/old-name   matches no skill (in @backend)
  · edit sets/backend.toml
! lint   skipped: ~/.claude/skills/lint
  is not in your vault · skl import lint
loaded (project ~/code/app)  data
  sql-explain   data/sql-explain
not in your vault
  my-new-skill  · skl import my-new-skill
skill sets      @backend  @base
vault           2 changes not pushed · skl sync
```

The project scope shows only inside a git repo that has skills loaded. `skl status` never goes online: `update available` is what the last [`skl sync`](./sync) found. For scripts, add `--json`: see [JSON](../reference/commands#json).

## Named skill sets

```sh
skl use @base review -review/test-gaps someone/pdf
skl save backend
```

saves the selectors behind your active skill set as `sets/backend.toml` in your vault, one selector per line:

```toml
select = [
  "@base",
  "review",
  "-review/test-gaps",
  "someone/pdf",
]
```

Selectors from later `add` and `drop` commands are appended. Because a skill set keeps selectors, not a list of skills, a skill you add to the `review` group later is part of `@backend` too. A skill set can include other skill sets.

If `backend` already exists, skl shows how its selectors change and asks before overwriting it.

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

## For one repo only

skl loads skills into your user folder, `~/.claude/skills`, by default. To load skills for one repo only:

```sh
skl use review --scope project
```

This copies them into `.claude/skills` at the root of the git repo you're in. Outside a git repo, `--scope project` is an error. See [Flags](../reference/commands#flags) for the commands that take it.

## Names and clashes

Claude Code reads skills from one flat folder, so skl copies each skill under its folder name: `review/pr-review` becomes `~/.claude/skills/pr-review`. Two skills clash when they have the same folder name or the same `name` in their `SKILL.md` frontmatter.

Between the skills you select, the later selector wins, and skl says which skill replaced which:

```sh
skl use review someone/pr-review
```

```
  ~ pr-review   someone/pr-review replaces review/pr-review
```

Claude Code prefers a user skill to a project skill with the same name. When a skill you loaded with `--scope project` is hidden that way, `skl status` warns about it.

## Skills skl didn't create

Skills you added by hand, installed with another tool such as [`npx skills`](../thirdparty/npx-skills), or installed through Claude Code plugins show up in `skl status` as not in your vault. skl never changes or removes them. Plugin skills never clash, because Claude Code prefixes their names with the plugin's.

If a hand-made skill already has a name that a loaded skill needs, skl skips its own skill, loads the rest, and warns you. The command still succeeds:

```
! pr-review   skipped: ~/.claude/skills/pr-review
  is not in your vault
  · skl import pr-review
```

`skl status` keeps listing it as skipped until the name is free.

`skl save` saves your selectors as they are; the skipped skills load once their names are free. To keep one in your vault, import it: see [Add a skill you made](./skills#add-a-skill-you-made).
