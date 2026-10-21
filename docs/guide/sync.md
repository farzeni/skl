# Sync

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Your vault is a plain git clone at `~/.skl/vault`. `skl sync` keeps it the same on every machine.

## What changes the vault

Commands that change the vault, such as `save`, `fork`, `learn`, `import` and `rollback`, commit right away on this machine. They never push.

You can also edit the vault by hand: change a `SKILL.md`, or edit or delete a file in `profiles/`. Those edits stay uncommitted until the next sync.

## What sync does

```sh
skl sync
```

1. **Commit.** Your hand edits become one commit, and skl lists the files in it.
2. **Pull.** skl fetches the vault and merges. It never rebases, so no commit is rewritten.
3. **Push.**
4. **Reload.** skl resolves what you have loaded again, for example `@backend writing/changelog`, and applies the difference. A profile changed on another machine, or a skill deleted there, takes effect here.

```
◇ committed  profiles/backend.toml, review/pr-review/SKILL.md
◇ pulled     3 commits from laptop
◇ pushed
  + data/sql-explain   − review/test-gaps
✓ generation 14
```

Sync creates a generation, so `skl rollback` undoes it like any other change.

## When both machines changed the same file

If the merge in step 2 conflicts, skl shows which files clash and what each side did, then asks how to resolve them:

```
✗ conflict   profiles/backend.toml
  this machine   drop review/test-gaps
  laptop         add data/sql-explain, use someone/agent-skills/test-gaps
? Resolve   › with Claude   by hand   abort
```

### With Claude

skl runs the `claude` CLI on the conflict, the same way [`skl learn`](./learn#who-does-the-thinking) does. Claude sees the version both sides started from, yours and the other machine's, plus both commit messages.

1. **Merge.** Claude combines the changes that don't contradict each other, and tells you what it did.
2. **Interview.** One question for each real conflict, about what you meant rather than which lines to keep.
3. **Review.** skl shows the merged result. You can ask for changes in plain words.
4. **Land.** skl checks the result (profiles must hold valid selectors, `SKILL.md` frontmatter must parse), commits the merge with your answers in its message, and carries on with the push.

```
◇ merged     data/sql-explain added (laptop only)
? This machine dropped test-gaps; laptop switched to someone/agent-skills/test-gaps. Which?
  › laptop's
◇ profiles/backend.toml   select = ["@base", "review", "data/sql-explain", "someone/agent-skills/test-gaps"]
? Land it   › yes   change it   abort
✓ merged · pushed · generation 14
```

This option shows only when the `claude` CLI is installed and skl runs in a terminal.

### By hand

skl leaves standard git conflict markers in the files in `~/.skl/vault`. Edit them, then finish with:

```sh
skl sync --continue
```

### Abort

Choose abort, or quit Claude at any point, and the merge is undone. Your hand edits stay committed on this machine, nothing is pushed and your loaded skills don't change. You can also run this at any point:

```sh
skl sync --abort
```
