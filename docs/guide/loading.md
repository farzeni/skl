# Loading skills

## Selectors

Every loading command takes the same selectors. They're applied left to right, and on a [name clash](#names-and-clashes) the later one wins.

| Selector | Loads |
|---|---|
| `review` | Every skill in one of your groups |
| `review/pr-review` | One skill from one of your groups |
| `@backend` | A saved profile |
| `owner/repo/skill` | A skill from someone else's repo, used read-only and pinned to a commit. Any git host works: see [Repo references](../reference/repos). |
| `-review/test-gaps` | Removes a skill that an earlier selector added |

```sh
skl use @backend -review/test-gaps anthropics/skills/pdf
```

## Changing the loaded set

| Command | Effect |
|---|---|
| `skl use <selectors>` | Replace the loaded set |
| `skl add <selectors>` | Add to it |
| `skl drop <selectors>` | Remove from it |
| `skl status` | Show what's loaded and where it came from |

Add `--dry-run` to any of them to see the changes without applying them.

## Profiles

A profile is a saved list of selectors, loaded with `@name`.

```sh
skl use @base review -review/test-gaps
skl save backend
```

saves the selectors behind what's loaded now as `profiles/backend.toml` in your vault, here `["@base", "review", "-review/test-gaps"]`. Selectors from later `add` and `drop` commands are appended. Because a profile keeps selectors, not a list of skills, a skill you add to the `review` group later is part of `@backend` too. A profile can include other profiles.

If `backend` already exists, skl shows how its selectors change and asks before overwriting it.

To change a profile, edit its file in `~/.skl/vault/profiles/`. To delete one, delete the file. `skl status` lists your profiles. Run `skl sync` to commit these edits and take them to your other machines: see [Sync](./sync).

## Scope

skl loads skills into your user folder, `~/.claude/skills`, by default. To load skills for one repo only:

```sh
skl use review --scope project
```

This links them into `./.claude/skills` in the current repo.

## Names and clashes

Claude Code reads skills from one flat folder, so skl links each skill under its folder name: `review/pr-review` becomes `~/.claude/skills/pr-review`. Two skills clash when they have the same folder name or the same `name` in their `SKILL.md` frontmatter.

Between the skills you select, the later selector wins, and skl says which skill replaced which:

```sh
skl use review someone/agent-skills/pr-review
```

```
  ~ pr-review   someone/agent-skills replaces review/pr-review
```

Claude Code prefers a user skill to a project skill with the same name. When a skill you loaded with `--scope project` is hidden that way, `skl status` warns about it.

## Skills skl didn't create

Skills you added by hand, or installed through Claude Code plugins, show up in `skl status` as not managed. skl never changes or removes them. Plugin skills never clash, because Claude Code prefixes their names with the plugin's.

If a hand-made skill already has a name that a loaded skill needs, skl skips its own skill, loads the rest, and exits with an error:

```
! pr-review   skipped: ~/.claude/skills/pr-review is not managed by skl
```

`skl status` keeps listing it as skipped until the name is free.

`skl save` leaves these skills out of the profile, and says which ones it left out. To keep one in your vault, import it:

```sh
skl import my-notes writing
```

This copies `~/.claude/skills/my-notes` into your `writing` group and commits it. The original stays where it is, so the copy clashes with it. Delete the original yourself, then load `writing/my-notes`.
