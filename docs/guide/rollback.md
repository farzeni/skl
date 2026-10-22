# Rollback

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Undo any change skl made or saved: a load, a lesson, an edit saved from Claude Code, a sync. Each of those is a **generation**, a numbered snapshot of your [active skill set](./skill-sets), meaning which skills are loaded where, plus the vault commit at that moment.

## See what changed

```sh
skl history
```

```
12  now      learn deep-pr-review → review/pr-review
11  2h ago   use @backend writing/changelog
10  3d ago   use @backend
```

## Go back

```sh
skl rollback       # one generation back
skl rollback 10    # to generation 10
```

A rollback restores what was loaded. If skill content changed in the meantime, for example a lesson replaced your skill, skl also adds a revert commit to your vault. It never rewrites git history.

## Undo a rollback

A rollback is itself a new generation, so you can undo it with another `skl rollback`.
