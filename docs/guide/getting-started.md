# Getting started

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Put your skills in a vault, load the ones you need, and have the same ones on your next machine.

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

`you/skills` is a GitHub repo, but any git remote works: see [Selectors and references](../reference/repos).

- If the repo has content, skl clones it to `~/.skl/vault`.
- If it's empty, skl sets up a new vault there and pushes it.
- If it doesn't exist and is on GitHub, skl offers to create it as a private repo with the `gh` CLI. On other hosts, create it first.

skl also writes this machine's settings to `~/.skl/config.toml`. See [Configuration](../reference/config).

## Bring in the skills you have

If you already have skills in `~/.claude/skills`, put them in your vault first. `skl status` lists them:

```
  pr-review     not in your vault · skl import pr-review <group>
  my-notes      not in your vault · skl import my-notes <group>
```

```sh
skl import pr-review review
skl import my-notes writing
```

A **group** is a folder of skills in your vault, such as `review` or `writing`. Claude Code sees no change. The skills are now versioned, and they go wherever your vault goes.

## Load only what you need

```sh
skl use review writing anthropics/skills/pdf
```

This loads every skill in your `review` and `writing` groups, and one skill from someone else's repo. skl copies them into `~/.claude/skills`, and Claude Code picks them up. The first time you load a skill from another repo, skl shows what it does and which scripts it runs, and asks before loading it.

```sh
skl status
```

shows what's loaded, where each skill comes from, and your named skill sets. [Skill sets](./skill-sets) explains why loading fewer skills helps Claude pick the right one.

## Save it as a skill set

```sh
skl save backend
```

saves what you just loaded as the skill set `@backend` in your vault. See [Named skill sets](./skill-sets#named-skill-sets) to change or delete one.

## Keep machines in sync

```sh
skl sync
```

commits your changes to the vault, pulls and pushes it, and reloads what you have loaded so it matches. Run it after changing skills on one machine, before switching to another. If two machines changed the same file, skl helps you merge them: see [Sync](./sync).

On a new machine, the same mix is two commands away:

```sh
skl init you/skills
skl use @backend
```

## Next

- [Your skills](./skills): keep improving skills in Claude Code; skl saves every change.
- [Rollback](./rollback): undo any change.
- [Skill sets](./skill-sets): load only what a task needs.
- [Other people's skills](./other-skills): use, copy and follow skills from any git repo.
- [Learn](./learn): improve a skill with what another one does better.
- [Sync](./sync): merge changes made on two machines.

Words like generation, peer or selector are defined in the [Glossary](../reference/glossary).
