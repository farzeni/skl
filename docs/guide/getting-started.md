# Getting started

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

## Install

skl is a single binary. It needs `git` on your `PATH`. Optional: the `claude` CLI for [lessons](./learn#how-a-lesson-works), and the `gh` CLI to create your vault repo on GitHub.

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
skl use review writing/changelog anthropics/skills/pdf
```

This loads every skill in your `review` group, one skill from your `writing` group and one skill from someone else's repo. skl copies them into `~/.claude/skills`, and Claude Code picks them up. The first time you load a skill from another repo, skl shows what it does and which scripts it runs, and asks before loading it.

```sh
skl status
```

shows what's loaded, where each skill comes from, and your named skill sets.

## Save it as a skill set

```sh
skl save backend
```

saves what you just loaded as the skill set `@backend` in your vault. On any other machine, after `skl sync`, the same mix is one command away:

```sh
skl use @backend
```

See [Named skill sets](./skill-sets#named-skill-sets) to change or delete one.

## Keep machines in sync

```sh
skl sync
```

commits your changes to the vault, pulls and pushes it, and reloads what you have loaded so it matches. Run it after changing skills on one machine, before switching to another. If two machines changed the same file, skl helps you merge them: see [Sync](./sync).

## Next

- [Concepts](./concepts): the six ideas skl is built on.
- [Your skills](./skills): make and improve skills in Claude Code, and keep them in your vault.
- [Skill sets](./skill-sets): selectors, named skill sets, scopes and name clashes.
- [Learn](./learn): improve your skills from other people's.
