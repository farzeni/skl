# Sync

::: warning Design preview
Parts of this page aren't built yet. See [What works in v0.1](/design/roadmap#what-works-in-v0-1).
:::

Keep the same vault, and the same loaded skills, on every machine, with changes made on both merged. Your vault is a plain git clone at `~/.skl/vault`.

## What sync does

```sh
skl sync
```

1. **Commit.** Your hand edits become one commit, changes to your loaded skills, in every repo, are saved back, and skl lists the files.
2. **Pull.** skl fetches the vault and merges. It never rebases, so no commit is rewritten.
3. **Push.**
4. **Reload.** skl resolves what you have loaded again, for example `@backend writing/changelog`, and applies the difference, in your user scope and the repo you're in. A skill set changed on another machine, or a skill deleted there, takes effect here. Another repo's skills reload when you run `skl sync` there: see [Work in a repo](./skill-sets#work-in-a-repo).
5. **Check for updates.** skl checks where the skills you learned came from. It changes nothing: `skl status` shows `update available`, and [`skl learn`](./learn#keep-up-with-its-author) takes it.

```
◇ committed  sets/backend.toml
◇ saved      pr-review → review/pr-review
◇ generation 13
◇ pulled     3 commits
◇ pushed
  + data/sql-explain   − someone/pdf
◇ checked    2 sources
! update available: someone/pdf · skl learn someone/pdf
✓ generation 14 · skl rollback to undo
```

Saving your changes first is a generation of its own, 13 here, and the sync is the next one. So `skl rollback` undoes the sync and keeps your saved changes, like any other change: see [Rollback](./rollback#see-what-changed). If the network fails, sync stops after step 1: your changes are saved and committed, and `skl status` lists them as not pushed. See [Errors](../reference/commands#errors).

## What gets committed

Commands that change the vault, such as `save`, `learn`, `import` and `rollback`, commit right away on this machine and never push. Changes you make to loaded skills, and hand edits in `~/.skl/vault` such as a `SKILL.md` or a file in `sets/`, are committed at the next command that changes something: see [Your skills](./skills). `skl sync` is what pushes them. skl never pushes on its own, so it reminds you instead: `skl status` and the last line of each command that changes something say what isn't pushed yet.

```
! 2 changes not pushed · skl sync
✓ generation 15 · skl rollback to undo
```

## When both machines changed the same file

If the merge in step 2 conflicts, skl shows which files clash and what each side did, then asks how to resolve them. Your loaded skills don't change until the merge lands, so Claude Code never sees a half-merged skill.

```
! conflict   sets/backend.toml
  this machine   drop someone/pdf
  other machine  add data/sql-explain
                 use someone/pdf-tools
? Resolve   › with Claude   by hand   abort
```

### With Claude

skl runs the [engine](./learn#who-does-the-thinking), the `claude` CLI by default, on the conflict, the same way [`skl learn`](./learn#who-does-the-thinking) does. Claude sees the version both sides started from, yours and the other machine's, plus both commit messages.

1. **Merge.** Claude combines the changes that don't contradict each other, and tells you what it did.
2. **Interview.** One question for each real conflict, about what you meant rather than which lines to keep.
3. **Review.** skl shows the merged result. You can ask for changes in plain words.
4. **Land.** skl checks the result (skill sets must hold valid selectors, `SKILL.md` frontmatter must parse), commits the merge with your answers in its message, and carries on with the push.

```
◇ merged     data/sql-explain added (other machine only)
? pdf: drop it (this machine)
  or use someone/pdf-tools (other machine)?
  › use someone/pdf-tools
◇ sets/backend.toml
    + data/sql-explain   + someone/pdf-tools
? Land it   › yes   change it   abort
◇ pushed
✓ generation 14 · skl rollback to undo
```

This option shows only when the engine's CLI is installed and skl runs in a terminal. With another [engine](./learn#other-agents) it carries the name of its command, such as `with opencode`. Without a terminal, or with `--yes`, sync undoes the merge and stops: see [Questions](../reference/commands#questions).

### By hand

skl leaves standard git conflict markers in the files in `~/.skl/vault`. Edit them, then finish with:

```sh
skl sync --continue
```

Any other edits you make in `~/.skl/vault` before `--continue` go into the merge commit, not into a generation of their own.

### Abort

Choose abort, or quit Claude at any point, and the merge is undone. Your hand edits stay committed on this machine, nothing is pushed and your loaded skills don't change. You can also run this at any point:

```sh
skl sync --abort
```
