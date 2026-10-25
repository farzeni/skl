# Your skills

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Keep improving your skills in Claude Code the way you do now. skl saves each change to your vault, so `skl sync` takes it to your other machines and `skl rollback` can undo it.

The examples from here on assume you've moved the skills you imported in [Getting started](./getting-started) into groups such as `review`, `data` and `writing`.

## Edit as usual

The skills skl loads are plain folders in `~/.claude/skills` (and `~/.agents/skills` with the [`agents` target](./skill-sets#which-agents-read-them)), made from the skills in your vault. Change one by hand, or ask Claude Code to improve it. When you're done, run `skl sync`: it saves your changes to the vault and takes them to your other machines. The other commands that change something (`use`, `add`, `drop`, `save`, `import`, `learn`, `rollback`) also save them first. `status` and `history` don't.

```
◇ saved      pr-review → review/pr-review
             SKILL.md, evals/evals.json
✓ generation 15 · skl rollback to undo
```

Each save is a **generation**, a numbered step in your [history](./rollback), so `skl rollback` undoes this save on its own. Until then, `skl status` lists the skills with unsaved changes.

skl saves only the files inside the skill's folder. Folders next to it, such as the `pr-review-workspace/` that skill-creator writes its test runs to, stay out of your vault.

## Add a skill you made

A skill you make in Claude Code, for example with the skill-creator skill, lands in `~/.claude/skills/<name>` (or `./.claude/skills/<name>` in a repo) and isn't in your vault yet. `skl status` lists it:

```
not in your vault
  my-new-skill   · skl import my-new-skill
```

```sh
skl import my-new-skill
```

copies it into your vault and commits it, in the group named after your vault's owner: `you/my-new-skill` for `you/skills`. Name several skills at once, or pick the group:

```sh
skl import changelog release-notes --group writing
```

The folder in `~/.claude/skills` stays where it is and becomes the loaded skill, so Claude Code sees no change. The skill joins your [active skill set](./skill-sets) as `skl add you/my-new-skill` would, unless a selector there already covers it, such as `you`. When `skl init` imports every skill into one group, it adds the group. skl looks for the folder in every [target](./skill-sets#which-agents-read-them)'s folder, and with two targets it also loads the skill into the other one. Run `skl sync` to take it to your other machines.

To import a skill from a repo's own `.claude/skills`, add `--scope project` inside that repo.

skl imports the folder as it is now. A name your vault already has, as a folder or a frontmatter `name`, stops the import:

```
✗ my-new-skill is already in your vault: you/my-new-skill
  · rename it in ~/.claude/skills, then skl import it
```

For what import does with a linked folder or a skill installed by another tool, see [Import](../design/architecture#import).

`skl init` offers the same for every skill it finds that isn't in a vault yet, when it sets up your vault.

## Rename or delete a skill

Rename or delete its folder in `~/.skl/vault`. The next command that loads skills copies the renamed skill in, or removes the deleted one. A selector that no longer matches anything, in a command or in a skill set, is a warning, not an error. `skl status` lists it:

```
! review/old-name   matches no skill (in @backend)
  · edit sets/backend.toml
```

## Edit the vault directly

You can also edit `~/.skl/vault/<group>/<skill>` directly. The next command that changes something, such as `skl sync`, commits your edits as a generation of their own and copies them into `~/.claude/skills`, where Claude Code sees them.

## When the vault changed too

If a skill changed in the vault after it was loaded, for example because `skl sync` pulled a change from another machine, and you changed the loaded skill too, skl merges both. When both changed the same lines, it asks you how to resolve them, the same way as [a sync conflict](./sync#when-both-machines-changed-the-same-file).

A skill you learned from someone else is in your vault like any other, so your changes are saved the same way. Their repo is never written. Once you've changed it, its next update from the author runs a [lesson](./learn#keep-up-with-its-author) instead of a plain update.
