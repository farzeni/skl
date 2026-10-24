# pi

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Run lessons and sync conflicts in [pi](https://pi.dev), the minimal coding agent (`@earendil-works/pi-coding-agent`), instead of Claude Code. Your skills need no part to load there.

## Your skills in pi

pi reads `~/.agents/skills`, `~/.pi/agent/skills`, and a repo's `.agents/skills` and `.pi/skills`, but not `~/.claude/skills`. Give it your skills one of two ways:

- Add the `agents` [target](../guide/skill-sets#which-agents-read-them) in `~/.skl/config.toml`: `targets = ["claude", "agents"]`, or `["agents"]` if you don't use Claude Code.
- Or point pi at skl's folder, in `~/.pi/agent/settings.json`:

  ```json
  { "skills": ["~/.claude/skills"] }
  ```

Not both: a skill in two folders pi reads loads once, with a warning about the clash.

pi doesn't see changes while it runs: run `/reload` in pi after `skl use`, `add`, `drop`, `sync` or `rollback`. It ignores frontmatter fields that only Claude Code knows, such as `context: fork`.

## Lessons in pi

```toml
# ~/.skl/config.toml
engine = "pi"
```

[Lessons](../guide/learn#who-does-the-thinking) and [sync conflicts](../guide/sync#with-claude) then start pi in the lesson workspace with skl's lesson skill and a first prompt: `pi --skill .agents/skills/<skill> "<prompt>"`. pi has no question dialog of its own, so the interview is a plain chat. pi uses its own login (`/login` in pi, or your provider's environment variables); skl never sees it. Without `pi` on your `PATH`, a lesson stops as it does without `claude`.

Not tested yet: a full lesson in pi. It's checked before lessons ship in v0.3: see [Roadmap](../design/roadmap).

## Without this part

Your skills load in pi as before. Only `engine = "pi"` stops working: a lesson stops before it starts, and `engine = "claude"` works as always.
