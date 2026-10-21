# Learn

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

You like how another skill handles something yours doesn't. `skl learn` lets your skill pick it up from that skill, its **peer**: Claude reads both, asks you what to take and how, and rewrites your skill. You review the diff before anything lands.

```sh
skl learn review/pr-review someone/agent-skills/deep-pr-review "I like its security checks"
```

The first argument is your skill. The second is its peer, given as any [repo reference](../reference/repos). You can leave the peer out to [learn from it again](#when-the-peer-changes). The text at the end is optional and goes to Claude as is, so say what you want in your own words. Without it, Claude asks.

## How it works

1. **Analyse.** Claude maps what both skills share, what only the peer does, what only yours does, and where they contradict each other.
2. **Interview.** One question for each decision: what you asked for, and every contradiction. Your answers are saved.
3. **Rewrite.** Claude rewrites your skill in its own voice and structure, adding the new capability. The peer's scripts come along only if you said yes.
4. **Review.** skl shows the diff. You can ask for changes in plain words ("make the security part shorter").
5. **Land.** You replace your skill, save the result under a new name, or discard it.

```
◇ peer only    OWASP checklist, secret scanning   ← what you asked for
◇ clash        peer comments inline; yours writes one summary
? Inline comments or one summary?
  › keep my summary, add a Security section
? Bring along the peer's secret-scanning script?
  › yes, as an optional step
◇ rewritten    SKILL.md +38 −5
? Land it   › replace   save as review/pr-review-sec   discard
✓ committed · generation 12
```

Landing creates a vault commit and a generation, so `skl rollback` undoes it. Run `skl sync` to push it.

To learn from more than one peer, run `skl learn` once for each. Every run is its own lesson, and you can roll each one back separately.

## When the peer changes

Peers keep improving. `skl upstream` lists your skills whose peer has new commits. Run `learn` again without a peer to pick up what's new:

```sh
skl learn review/pr-review "only the new security checks"
```

skl uses the peer you learned from last. For a [fork](./fork), the peer is its source. Claude sees three versions: the peer as you last learned from it, the peer now, and your skill. So it can tell the peer's changes from yours, and asks only about what's new. It reuses your saved answers, and changes you skip aren't asked about again.

## Lessons

Each run is recorded in the skill's `.skl-origin.toml` as a lesson. It holds the peer, its commit, what you asked for, your answers and what you declined.

```toml
[[lesson]]
peer     = "https://github.com/someone/agent-skills.git//deep-pr-review"
commit   = "7c21e9f"
prompt   = "I like its security checks"
answers  = "lessons/7c21e9f.md"
declined = ["inline comments"]
```

## Options

| Option | Effect |
|---|---|
| `"<text>"` | What you want, in your own words. Optional. |
| `--as <name>` | Skip the landing question and save under a new name. |
| `--all` | When learning again, also ask about changes you skipped before. |

## Who does the thinking

skl prepares a workspace with both skills and runs the `claude` CLI in it, using your existing login. Claude writes its result to a staging folder. Only skl commits to your vault, and only after you choose.
