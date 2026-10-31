# Getting started

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

In ten minutes your skills are in a git repo you own, safe to edit and undo, and one command away on your next machine.

## Install

```sh
go install github.com/farzeni/skl/cmd/skl@latest
```

skl is a single binary. It needs git 2.38 or later; without git, it stops with `✗ skl needs git · install git 2.38 or later`. [Lessons](./learn) need an agent to run them: the `claude` CLI by default, or another agent's, set as the [engine](./learn#other-agents).

The examples use Claude Code. For opencode, pi, Codex and other agents, see [Which agents read them](./skill-sets#which-agents-read-them).

## Make your vault

Your **vault** is a git repo that holds your skills. Use the same one on every machine. Create an empty repo for it on your git host first, private if you like, then run:

```sh
skl init you/skills
```

```
→ new vault you/skills
? Import the 3 skills in ~/.claude/skills into you/?
  › yes   choose   no
◇ imported   pr-review, my-notes, sql-explain → you/
◇ LICENSE    MIT, for the skills you write
  · edit LICENSE to change it
! 1 change not pushed · skl sync
✓ generation 1 · skl rollback to undo
```

Your skills stay where Claude Code reads them. They're now versioned, and they go wherever your vault goes. They sit in a **group** named after you, `you/`; you can [move them](./skills#rename-or-delete-a-skill) into groups such as `review` or `writing` later.

If the repo already holds skills, skl clones it as your vault instead: its groups are the folders that hold skills. An empty repo becomes a new vault. skl never creates a repo, so one it can't reach stops init:

```
✗ can't reach you/skills
  · create it or check access, then skl init you/skills
```

Any git remote works: see [Selectors and references](../reference/repos). To pick which skills to import, answer `choose`: skl asks about each one.

Running `skl init` again with the same repo only loads the selectors you give it. A machine has one vault, so a different repo stops:

```
✗ ~/.skl/vault already holds you/skills
  · SKL_HOME=~/.skl-team skl init team/skills
```

[`SKL_HOME`](../reference/config#environment-variables) keeps a second setup apart.

## Work as usual

Make and improve skills in Claude Code, by hand or by asking Claude, as you do now. Say you've sharpened `pr-review` and made a new skill, `my-new-skill`. `skl status` shows where things stand:

```sh
skl status
```

```
loaded (user)  you
  pr-review     you/pr-review    changed · skl sync
  my-notes      you/my-notes
  sql-explain   you/sql-explain
not in your vault
  my-new-skill  · skl import my-new-skill
vault           1 change not pushed · skl sync
```

Every line that needs you ends with the command to run. `changed` means your edits to `pr-review` aren't in the vault yet. They get there at the start of the next skl command that changes something, such as `import`, `learn` or `sync`. `skl status` itself changes nothing.

A skill you make in Claude Code isn't in your vault until you import it:

```sh
skl import my-new-skill
```

```
◇ saved      pr-review → you/pr-review
             SKILL.md
◇ generation 2
◇ imported   my-new-skill → you/
! 3 changes not pushed · skl sync
✓ generation 3 · skl rollback to undo
```

skl saved your `pr-review` edits first, as a generation of their own, then imported the new skill. Each change is a numbered **generation** you can go back to. All of it is committed in your vault on this machine, not pushed. When you're done, push:

```sh
skl sync
```

`skl sync` saves any edits left and pushes them. See [Your skills](./skills) for renaming and deleting skills, and [What's loaded](./skill-sets#what-s-loaded) for every kind of status line.

## Get a skill from someone

```sh
skl learn someone/agent-skills/pdf
```

```
→ copy into someone/pdf
  Extract text, tables and forms from PDF files.
  license    Apache-2.0 · notice kept in someone/pdf/LICENSE
  scripts    scripts/extract.py
? Add it?   › yes   no
+ pdf
! 1 change not pushed · skl sync
✓ generation 4 · skl rollback to undo
```

skl shows what someone else's skill does and what it runs before it lands. The skill is now in your vault and loaded, with its source recorded. When its author changes it, `skl status` says so and `skl learn someone/pdf` takes the change. See [Learn](./learn#get-a-skill), which also covers `skl search`.

Changed your mind? `skl rollback` takes back the last generation: see [Rollback](./rollback).

## Teach your skill a trick

Your `pr-review` is fine, but someone else's checks security better:

```sh
skl learn someone/agent-skills/deep-pr-review \
  you/pr-review "its security checks"
```

Claude reads both skills, asks you what to take, and shows the diff before anything lands. See [Teach your skill a trick](./learn#teach-your-skill-a-trick).

## Load only what a task needs

Claude Code shows Claude the description of every loaded skill, and past a budget it cuts them short. Load the groups and skills a task needs, and save the mix:

```sh
skl use you someone/pdf -you/my-notes
skl save backend
```

`@backend` now loads the same mix on any machine: `skl use @backend`. See [Skill sets](./skill-sets).

A repo can load more skills, for itself only. Inside it, add `--scope project`:

```sh
cd ~/code/prj1
skl use you/my-notes --scope project
```

Claude Code sees `my-notes` only in `prj1`, next to your `@backend` mix. Each repo has its own active skill set, which you can save like any other: see [For one repo only](./skill-sets#for-one-repo-only).

## On your next machine

Here, push your changes:

```sh
skl sync
```

There, get your vault and load your mix, then each repo's own skills:

```sh
skl init you/skills @backend
cd ~/code/prj1 && skl use you/my-notes --scope project
```

Run `skl sync` whenever you switch machines: it pushes this machine's changes and pulls the others'. If two machines changed the same skill, see [Sync](./sync).

## Next

- [Your skills](./skills): import, rename and delete skills, or edit the vault directly.
- [Rollback](./rollback): see your generations and go back to any of them.
- [Skill sets](./skill-sets): per-repo skills and name clashes.
- [Learn](./learn): get skills, teach yours, keep up with authors, licenses.
- [Sync](./sync): when two machines changed the same file.

Words like generation, peer or selector are defined in the [Glossary](../reference/glossary).
