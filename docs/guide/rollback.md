# Rollback

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Every command that changes something creates a **generation**: a numbered snapshot of your [active skill set](./skill-sets), meaning which skills are loaded where, plus the vault commit at that moment. That covers `use`, `add`, `drop`, `save`, `import`, `learn` and `sync`, and saving back changes from a loaded skill. So rolling back is how you take a skill set, and the skills in it, back to an earlier state.

```sh
skl history
```

```
12  now      learn deep-pr-review → review/pr-review
11  2h ago   use @backend writing/changelog
10  3d ago   use @backend
```

## Going back

```sh
skl rollback       # one generation back
skl rollback 10    # to generation 10
```

A rollback restores the loaded copies. If skill content changed in the meantime, for example a lesson replaced your skill, skl also adds a revert commit to your vault. It never rewrites git history.

A rollback is itself a new generation, so you can undo it with another `skl rollback`.
