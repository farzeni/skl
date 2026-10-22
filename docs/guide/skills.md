# Your skills

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Keep improving your skills in Claude Code the way you do now. skl saves each change to your vault, so `skl sync` takes it to your other machines and `skl rollback` can undo it.

## Edit as usual

The skills skl loads are plain folders in `~/.claude/skills`, copies of the skills in your vault. Change one by hand, or ask Claude Code to improve it. When you're done, run `skl sync`: it saves your changes to the vault and takes them to your other machines. The other commands that change something (`use`, `add`, `drop`, `save`, `import`, `learn`, `rollback`) also save them first. `status` and `history` don't.

```
◇ saved back   pr-review → review/pr-review
               SKILL.md, evals/evals.json
✓ generation 15
```

Each save is a **generation**, a numbered step in your [history](./rollback), so `skl rollback` undoes this save on its own. Until then, `skl status` lists the skills with unsaved changes.

skl saves only the files inside the skill's folder. Folders next to it, such as the `pr-review-workspace/` that skill-creator writes its test runs to, stay out of your vault.

## Add a skill you made

A skill you make in Claude Code, for example with the skill-creator skill, lands in `~/.claude/skills/<name>` (or `./.claude/skills/<name>` in a repo) and isn't in your vault yet. `skl status` lists it:

```
  my-notes      not in your vault · skl import my-notes <group>
```

```sh
skl import my-notes writing
```

copies it into your `writing` group and commits it. The folder in `~/.claude/skills` stays where it is and becomes skl's copy of `writing/my-notes`, so Claude Code sees no change. Run `skl sync` to take it to your other machines.

## Rename or delete a skill

Rename or delete its folder in `~/.skl/vault`. The next command that loads skills copies the renamed skill in, or removes the deleted one. A selector that no longer matches anything, in a command or in a skill set, is a warning, not an error. `skl status` lists it:

```
! review/old-name   matches no skill   (in @backend)
```

## Edit the vault directly

You can also edit `~/.skl/vault/<group>/<skill>` directly. The next command that changes something, such as `skl sync`, commits your edits as a generation of their own and copies them into `~/.claude/skills`, where Claude Code sees them.

## When the vault changed too

If a skill changed in the vault after it was copied, for example because `skl sync` pulled a change from another machine, and you changed the copy too, skl merges both. When both changed the same lines, it asks you how to resolve them, the same way as [a sync conflict](./sync#when-both-machines-changed-the-same-file).

A skill you learned from someone else is in your vault like any other, so your changes are saved the same way. Their repo is never written. Once you've changed it, its next update from the author runs a [lesson](./learn#when-the-peer-changes) instead of a plain update.
