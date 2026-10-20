# Learn

You like how another skill handles something your skill doesn't. `skl learn` lets your skill pick it up: Claude reads both skills, asks you what to take and how, and rewrites your skill. You review the diff before anything lands.

```sh
skl learn someone/agent-skills/deep-pr-review --into review/pr-review --focus security
```

## How it works

1. **Analyse.** Claude maps what both skills share, what only the donor does, what only yours does, and where they contradict each other.
2. **Interview.** One question for each decision: your focus, and every contradiction. Your answers are saved.
3. **Rewrite.** Claude rewrites your skill in its own voice and structure, adding the new capability. Donor scripts come along only if you said yes.
4. **Review.** skl shows the diff. You can ask for changes in plain words ("make the security part shorter").
5. **Land.** You replace your skill, save the result under a new name, or discard it.

```
◇ donor only   OWASP checklist, secret scanning   ← your focus
◇ clash        donor comments inline; yours writes one summary
? Inline comments or one summary?
  › keep my summary, add a Security section
? Bring along the donor's secret-scanning script?
  › yes, as an optional step
◇ rewritten    SKILL.md +38 −5
? Land it   › replace   save as review/pr-review-sec   discard
✓ committed · generation 12
```

Landing creates a vault commit and a generation, so `skl rollback` undoes it. Run `skl sync` to push it.

## Lessons

Each run is recorded in the skill's `.skl-origin.toml` as a lesson: the donor, its commit, your focus and your answers.

```toml
[[lesson]]
donor   = "someone/agent-skills/deep-pr-review"
commit  = "7c21e9f"
focus   = "security"
answers = "lessons/7c21e9f.md"
```

When a donor changes, `skl upstream` tells you. Running `skl learn --again` reuses your saved answers and asks only about what's new.

## Options

| Option | Effect |
|---|---|
| `--into <skill>` | The skill to improve. Required. |
| `--focus <text>` | What you want from the donor. Without it, Claude asks. |
| `--from <ref>` | Add more donors, e.g. security from one and performance from another, in one interview. |
| `--as <name>` | Skip the landing question and save under a new name. |
| `--again` | Re-run a previous lesson with your saved answers. |

## Who does the thinking

skl prepares a workspace with both skills and runs the `claude` CLI in it, using your existing login. Claude writes its result to a staging folder. Only skl commits to your vault, and only after you choose.
