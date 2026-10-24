# opencode

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Run lessons and sync conflicts in [opencode](https://opencode.ai) instead of Claude Code. Your skills need no part to load there: opencode already reads the folder skl writes.

## Your skills in opencode

opencode reads `~/.claude/skills` and a repo's `.claude/skills`, so every skill skl loads with the default `claude` [target](../guide/skill-sets#which-agents-read-them) is there. It also reads `~/.agents/skills` and `~/.config/opencode/skills`.

- A skill in `~/.agents/skills` wins over one with the same name in `~/.claude/skills`. `skl status` warns when that hides a skill you loaded: see [Skills skl didn't create](../guide/skill-sets#skills-skl-didn-t-create).
- For opencode alone, keep one target. With both, opencode sees each skill twice and keeps one.
- opencode doesn't see changes while it runs. Restart it after `skl use`, `add`, `drop`, `sync` or `rollback`.
- `OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1` stops opencode reading the `.claude` folders. Use the `agents` target then.

## Lessons in opencode

```toml
# ~/.skl/config.toml
engine = "opencode"
```

[Lessons](../guide/learn#who-does-the-thinking) and [sync conflicts](../guide/sync#with-claude) then start opencode in the lesson workspace with a first prompt, `opencode . --prompt "<prompt>"`, and opencode finds skl's lesson skill in the workspace's `.agents/skills`. It asks its questions in its own interface and uses its own login (`opencode auth login`, or your provider's environment variables); skl never sees it. Without `opencode` on your `PATH`, a lesson stops as it does without `claude`.

Not tested yet: a full lesson in opencode, and whether `--prompt` keeps opencode open for the interview. Both are checked before lessons ship in v0.3: see [Roadmap](../design/roadmap).

## Without this part

Your skills load in opencode as before. Only `engine = "opencode"` stops working: a lesson stops before it starts, and `engine = "claude"` works as always.
