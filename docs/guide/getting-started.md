# Getting started

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

In ten minutes your skills are in a git repo you own, safe to edit and undo, and one command away on your next machine.

## Install

```sh
go install github.com/farzeni/skl/cmd/skl@latest
```

skl is a single binary. It needs `git`. [Lessons](./learn) need the `claude` CLI, and creating your vault repo on GitHub needs the `gh` CLI.

## Make your vault

Your **vault** is a git repo that holds your skills. Use the same one on every machine.

```sh
skl init you/skills
```

```
→ new vault you/skills (private, on GitHub)
? Import the 3 skills in ~/.claude/skills into you/?
  › yes   choose   no
◇ imported   pr-review, my-notes, sql-explain → you/
◇ LICENSE    MIT, for the skills you write · edit LICENSE to change it
✓ generation 1 · skl rollback to undo
```

Your skills stay where Claude Code reads them. They're now versioned, and they go wherever your vault goes. They sit in a **group** named after you, `you/`; you can [move them](./skills#rename-or-delete-a-skill) into groups such as `review` or `writing` later.

If the repo already holds a vault, skl clones it instead. If it's missing and on GitHub, skl offers to create it as a private repo. Any git remote works: see [Selectors and references](../reference/repos).

## Keep editing as usual

Improve your skills in Claude Code, by hand or by asking Claude. skl saves your changes to the vault at the start of the next skl command that changes something. Each change is a numbered **generation**, and any of them can be undone:

```sh
skl history
skl rollback        # one step back
skl rollback 4      # back to generation 4
```

A skill you make later in Claude Code isn't in your vault yet. `skl status` lists it with the command to add it: `skl import my-new-skill`. See [Your skills](./skills).

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
✓ generation 2 · skl rollback to undo
```

skl shows what someone else's skill does and what it runs before it lands. The skill is now in your vault and loaded, with its source recorded. When its author changes it, `skl status` says so and `skl learn someone/pdf` takes the change. See [Get skills](./get-skills), which also covers `skl search`.

## Teach your skill a trick

Your `pr-review` is fine, but someone else's checks security better:

```sh
skl learn someone/agent-skills/deep-pr-review \
  you/pr-review "its security checks"
```

Claude reads both skills, asks you what to take, and shows the diff before anything lands. See [Learn](./learn).

## Load only what a task needs

Claude Code shows Claude the description of every loaded skill, and past a budget it cuts them short. Load the groups and skills a task needs, and save the mix:

```sh
skl use you someone/pdf -you/my-notes
skl save backend
```

`@backend` now loads the same mix on any machine: `skl use @backend`. See [Skill sets](./skill-sets).

## See where you stand

```sh
skl status
```

```
loaded (user)  @backend
  you/pr-review   changed · saved at the next command
  someone/pdf     update available · skl learn someone/pdf
not in your vault
  my-new-skill    · skl import my-new-skill
vault           2 changes not pushed · skl sync
```

Every line that needs you ends with the command to run.

## On your next machine

Here, push your changes:

```sh
skl sync
```

There, get your vault and load your mix:

```sh
skl init you/skills @backend
```

Run `skl sync` whenever you switch machines: it pushes this machine's changes and pulls the others'. If two machines changed the same skill, see [Sync](./sync).

## Next

- [Your skills](./skills): import, rename and delete skills, or edit the vault directly.
- [Rollback](./rollback): go back to any generation.
- [Skill sets](./skill-sets): per-repo skills and name clashes.
- [Get skills](./get-skills): find skills, keep up with their authors, licenses.
- [Learn](./learn): how a lesson works.
- [Sync](./sync): when two machines changed the same file.

Words like generation, peer or selector are defined in the [Glossary](../reference/glossary).
