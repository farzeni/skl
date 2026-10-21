# Fork

A fork copies one skill from someone else's repo into your vault, and remembers where it came from.

```sh
skl fork anthropics/skills/pdf          # into a group named after the source
skl fork anthropics/skills/pdf docs     # into your docs group
```

The copy is yours to edit. skl records the source repo and commit in `.skl-origin.toml` next to the skill.

## Getting the author's changes

```sh
skl upstream
```

```
docs/pdf     anthropics/skills   4 commits behind   you changed 2 files
```

There are two ways to take them.

### Learn: take what you choose

You've usually changed your fork, and want only part of what the author changed. A fork's source is its peer, so [learn](./learn#when-the-peer-changes) from it: Claude sorts the author's changes from yours and asks you about each one.

```sh
skl learn docs/pdf "take the faster table extraction, keep my CSV output"
```

### Merge: take everything

```sh
skl upstream docs/pdf --merge
```

skl rebuilds three versions of the skill: the commit you forked from, your copy, and the author's latest. It merges each file with `git merge-file`. If both sides changed the same lines, the file gets standard conflict markers. Edit it, then finish with:

```sh
skl upstream --continue
```

## Fork or learn?

Fork when you want the whole skill and plan to follow its author, taking their later changes with `skl learn` or `skl upstream --merge`. [Learn](./learn) when you want one thing a skill does well, inside a skill you already have.
