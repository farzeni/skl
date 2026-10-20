# Getting started

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

## Install

skl is a single binary. It needs `git` on your `PATH`, plus the `claude` CLI if you want to use [`skl learn`](./learn).

```sh
go install github.com/farzeni/skl/cmd/skl@latest
```

## Create or clone your vault

Your vault is a git repo that holds your skills. On your first machine, point skl at an empty repo; on every other machine, point it at the same one.

```sh
skl init you/skills
```

The vault is cloned to `~/.skl/vault`.

## Load skills

```sh
skl use @backend writing/changelog anthropics/skills/pdf
```

This loads a saved profile, one skill from your own `writing` group and one skill from someone else's repo. skl links them into `~/.claude/skills`, and Claude Code picks them up. The first time you load a skill from another repo, skl shows what it does and which scripts it runs, and asks before loading it.

```sh
skl status
```

shows what's loaded and where each skill comes from.

## Keep machines in sync

```sh
skl sync
```

pulls and pushes your vault. Run it after changing skills on one machine, before switching to another.

## Next

- [Concepts](./concepts): the six ideas skl is built on.
- [Loading skills](./loading): selectors, profiles and scopes.
- [Learn](./learn): improve your skills from other people's.
