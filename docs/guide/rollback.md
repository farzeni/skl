# Rollback

::: warning Design preview
Parts of this page aren't built yet. See [What works in v0.1](/design/roadmap#what-works-in-v0-1).
:::

Undo any change skl made or saved: a load, a lesson, an edit saved from Claude Code, a sync. Each of those is a **generation**, a numbered snapshot of your [active skill set](./skill-sets), meaning which skills are loaded where, plus the vault commit at that moment.

## See what changed

```sh
skl history
```

```
12  now      learn deep-pr-review → review/pr-review
11  2h ago   use @backend writing/changelog
10  2h ago   saved pr-review → review/pr-review
 9  3d ago   use @backend
```

`skl history` lists every generation, newest first. `--json` prints the same list for scripts: see [JSON](../reference/commands#json).

A command that first [saves your edits](./skills#edit-as-usual) makes two generations: the save, then its own. Generation 10 above is such a save, so `skl rollback 10` undoes generations 11 and 12 and keeps the edits saved in 10. A command that changes nothing makes no generation. See [Generations](../design/architecture#generations) for how they're kept.

Generations are numbered on each machine. Your laptop's generation 12 isn't your desktop's: the history is local, and only the vault is synced.

## Go back

```sh
skl rollback       # one generation back
skl rollback 10    # to generation 10
```

```
→ back to generation 10
  − writing/changelog
◇ reverted   review/pr-review (revert commit 5d0e7aa)
✓ generation 13 · skl rollback to undo
```

A rollback restores what was loaded. If skill content changed in the meantime, for example a lesson replaced your skill, skl also adds a revert commit to your vault. It never rewrites git history. Add `--dry-run` to see the plan without applying it.

A rollback restores every scope the generations it undoes changed: your user scope, and the [project scope](./skill-sets#for-one-repo-only) of any repo they ran in, wherever you run it. `skl history` shows that repo after the command, as in `use @frontend · ~/code/prj1`. A repo that no longer exists is skipped.

If the generations you go back over include changes pulled by `skl sync`, the revert takes back the other machine's work too, and your next `skl sync` pushes that. skl warns you when it applies the plan:

```
! reverts 3 commits from laptop · skl rollback to undo
```

There's nothing to go back to before generation 1, and `N` must be a generation in `skl history`:

```
✗ no generation 40 · skl history
```

## Undo a rollback

A rollback is itself a new generation, so you can undo it with another `skl rollback`.
