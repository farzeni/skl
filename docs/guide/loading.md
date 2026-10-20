# Loading skills

## Selectors

Every loading command takes the same selectors. They're applied left to right, and on a name clash the later one wins.

| Selector | Loads |
|---|---|
| `review` | Every skill in one of your groups |
| `review/pr-review` | One skill from one of your groups |
| `@backend` | A saved profile |
| `owner/repo/skill` | A skill from someone else's repo, used read-only and pinned to a commit |
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

```sh
skl save backend
```

saves the loaded set as `profiles/backend.toml` in your vault. Load it anywhere with `skl use @backend`. A profile can include other profiles.

## Scope

skl loads skills into your user folder, `~/.claude/skills`, by default. To load skills for one repo only:

```sh
skl use review --scope project
```

This links them into `./.claude/skills` in the current repo.

## Skills skl didn't create

Skills you added by hand, or installed through Claude Code plugins, show up in `skl status` as not managed. skl never changes or removes them.
