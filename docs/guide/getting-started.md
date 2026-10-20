# Getting started

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

## Install

skl is a single binary. It needs `git` on your `PATH`. Optional: the `claude` CLI for [`skl learn`](./learn), and the `gh` CLI to create your vault repo on GitHub.

```sh
go install github.com/farzeni/skl/cmd/skl@latest
```

## Set up your vault

Your vault is a git repo that holds your skills. Use the same one on every machine.

```sh
skl init you/skills
```

`you/skills` is a GitHub repo, but any git remote works: see [Repo references](../reference/repos).

- If the repo has content, skl clones it to `~/.skl/vault`.
- If it's empty, skl sets up a new vault there and pushes it.
- If it doesn't exist and is on GitHub, skl offers to create it as a private repo with the `gh` CLI. On other hosts, create it first.

skl also writes this machine's settings to `~/.skl/config.toml`. See [Configuration](../reference/config).

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
