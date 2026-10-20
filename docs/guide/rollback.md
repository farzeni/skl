# Rollback

Every command that changes something creates a **generation**: a numbered snapshot of which skills are linked where, plus the vault commit at that moment. That covers `use`, `add`, `drop`, `learn`, `fork` and `sync`.

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

A rollback restores the links. If skill content changed in the meantime, for example a lesson replaced your skill, skl also adds a revert commit to your vault. It never rewrites git history.

A rollback is itself a new generation, so you can undo it with another `skl rollback`.
