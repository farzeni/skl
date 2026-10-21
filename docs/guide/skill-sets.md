# Skill sets

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

A skill set is a list of selectors: the groups and skills you load together. The one you have loaded now is your **active skill set**. Save it with a name and it becomes a **named skill set**, which you load anywhere with `@name`.

A group is a folder in your vault, where a skill lives. A skill set is a list of selectors, what you load.

## Selectors

Every loading command takes the same selectors. They're applied left to right, and on a [name clash](#names-and-clashes) the later one wins.

| Selector | Loads |
|---|---|
| `review` | Every skill in one of your groups |
| `review/pr-review` | One skill from one of your groups |
| `@backend` | A named skill set from your vault |
| `owner/repo/skill` | A skill from someone else's repo, used read-only and pinned to a commit. Any git host works: see [Repo references](../reference/repos). |
| `owner/repo/@backend` | A skill set from someone else's repo, used read-only. Its selectors are read inside that repo, so `review` there means their `review` group. |
| `-review/test-gaps` | Removes a skill that an earlier selector added |

```sh
skl use @backend -review/test-gaps anthropics/skills/pdf
```

skl's flags start with `--` and can go anywhere on the line. Every other word is a selector, so `-review/test-gaps` is never read as a flag.

## Changing the active skill set

| Command | Effect |
|---|---|
| `skl use <selectors>` | Replace the active skill set |
| `skl add <selectors>` | Add to it |
| `skl drop <selectors>` | Remove from it |
| `skl status` | Show what's loaded, where it came from, and your named skill sets |

Add `--dry-run` to any of them to see the changes without applying them.

Loaded skills are plain copies. Changes you make to them are saved back to your vault: see [Your skills](./skills).

## Named skill sets

```sh
skl use @base review -review/test-gaps
skl save backend
```

saves the selectors behind your active skill set as `sets/backend.toml` in your vault, one selector per line:

```toml
select = [
  "@base",
  "review",
  "-review/test-gaps",
]
```

Selectors from later `add` and `drop` commands are appended. Because a skill set keeps selectors, not a list of skills, a skill you add to the `review` group later is part of `@backend` too. A skill set can include other skill sets.

If `backend` already exists, skl shows how its selectors change and asks before overwriting it.

To change a named skill set, edit its file in `~/.skl/vault/sets/`. To delete one, delete the file. `skl status` lists your named skill sets. Run `skl sync` to commit these edits and take them to your other machines: see [Sync](./sync).

## Scope

skl loads skills into your user folder, `~/.claude/skills`, by default. To load skills for one repo only:

```sh
skl use review --scope project
```

This copies them into `./.claude/skills` in the current repo.

## Names and clashes

Claude Code reads skills from one flat folder, so skl copies each skill under its folder name: `review/pr-review` becomes `~/.claude/skills/pr-review`. Two skills clash when they have the same folder name or the same `name` in their `SKILL.md` frontmatter.

Between the skills you select, the later selector wins, and skl says which skill replaced which:

```sh
skl use review someone/agent-skills/pr-review
```

```
  ~ pr-review   someone/agent-skills replaces review/pr-review
```

Claude Code prefers a user skill to a project skill with the same name. When a skill you loaded with `--scope project` is hidden that way, `skl status` warns about it.

## Skills skl didn't create

Skills you added by hand, installed with another tool such as [`npx skills`](../thirdparty/npx-skills), or installed through Claude Code plugins show up in `skl status` as not managed. skl never changes or removes them. Plugin skills never clash, because Claude Code prefixes their names with the plugin's.

If a hand-made skill already has a name that a loaded skill needs, skl skips its own skill, loads the rest, and exits with an error:

```
! pr-review   skipped: ~/.claude/skills/pr-review is not managed by skl
```

`skl status` keeps listing it as skipped until the name is free.

`skl save` leaves these skills out of the skill set, and says which ones it left out. To keep one in your vault, import it:

```sh
skl import my-notes writing
```

This copies `~/.claude/skills/my-notes` into your `writing` group and commits it. The original becomes skl's copy of `writing/my-notes`: see [Make a skill](./skills#make-a-skill).
