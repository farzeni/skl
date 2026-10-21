# Your skills

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

You make and improve skills in Claude Code, the way you would without skl. skl keeps the result in your vault.

## Make a skill

Ask Claude Code to make one, for example with the skill-creator skill, or create the folder yourself. Either way it lands in `~/.claude/skills/<name>` (or `./.claude/skills/<name>` in a repo). Until you import it, `skl status` lists it as not managed:

```
  my-notes      not managed by skl · skl import my-notes <group>
```

```sh
skl import my-notes writing
```

copies it into your `writing` group and commits it. The folder in `~/.claude/skills` stays where it is and becomes skl's copy of `writing/my-notes`, so nothing moves and nothing clashes. Run `skl sync` to take it to your other machines.

## Improve a skill

Every skill skl loads is a plain folder in `~/.claude/skills`, a copy of the skill in your vault. Change it there, by hand or by asking Claude Code. skl saves your changes back to the vault at the next command that changes something, `skl sync` included:

```
◇ saved back   pr-review → review/pr-review   SKILL.md, evals/evals.json
✓ generation 15
```

Saving back is its own generation, so `skl rollback` can undo it on its own. Until then, `skl status` lists the copies with changes that aren't saved yet.

Only the files inside the skill's folder are saved. Folders next to it stay where they are, such as the `pr-review-workspace/` that skill-creator writes its test runs to.

If the skill also changed in the vault since it was copied, for example because `skl sync` pulled a change from another machine, skl merges both. When both changed the same lines, it asks you how to resolve them, the same way as [a sync conflict](./sync#when-both-machines-changed-the-same-file).

## Edit in the vault

You can also edit `~/.skl/vault/<group>/<skill>` directly. Claude Code sees those edits after the next command that changes something, such as `skl sync`, copies them in.

Before any command that changes the vault, skl commits your hand edits there as a generation of their own.

## Rename or delete a skill

Rename or delete its folder in `~/.skl/vault`. The next command that loads skills copies the renamed skill in, or removes the deleted one. A selector that no longer matches anything, in a command or in a profile, is a warning, not an error. `skl status` lists it:

```
! review/old-name   matches no skill   (in @backend)
```

## Someone else's skill

Skills from other people's repos are loaded as copies too, and skl never writes your changes into their repo or its cache. If you change one, skl stops before it refreshes or reloads that skill, and asks what to do:

```
? pdf changed since it was copied from anthropics/skills @3f9a1c2
  › fork it with my changes   discard my changes   keep for now
```

- **Fork it** copies the skill as of the pinned commit into your vault, adds your changes, and records its source as [`skl fork`](./fork) does, so `skl upstream` keeps working.
- **Discard** puts the pinned version back.
- **Keep for now** leaves your copy as it is. skl doesn't refresh it, and `skl status` flags it. Outside a terminal, this is what happens.
