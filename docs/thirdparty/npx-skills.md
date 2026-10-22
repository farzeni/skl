# npx skills

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

[`npx skills`](https://github.com/vercel-labs/skills) is Vercel's installer for agent skills. It works next to skl without any setup.

## Skills it installed

`npx skills add -g` puts a skill in `~/.claude/skills/<name>`, either as a link into `~/.agents/skills/` or as a plain folder. To skl it's a [skill skl didn't create](../guide/skill-sets#skills-skl-didn-t-create): skl leaves it alone, and skips its own skill if the names clash.

To keep such a skill in your vault, [import](../guide/skills#add-a-skill-you-made) it:

```sh
skl import pdf --group docs
```

`npx skills` records where each skill came from in `~/.agents/.skill-lock.json`. If the skill is listed there with a git source, `skl import` records that source too, as a [copy made by `skl learn`](../guide/learn#get-a-skill) does, so [`skl status`](../guide/learn#keep-up-with-its-author) shows the author's later changes:

```
◇ imported   pdf → docs/pdf
  source     someone/agent-skills/skills/pdf (npx skills)
! pdf is also tracked by npx skills: `npx skills update` can
  change this folder and `npx skills remove` deletes it.
  Your vault keeps the imported version.
✓ generation 15 · skl rollback to undo
```

If the lock file is missing, or in a format skl doesn't know, `skl import` makes a plain copy. skl never changes the lock file or `~/.agents/`.

## Installing from your vault

Your vault is a valid source for `npx skills`, as it is:

```sh
npx skills add you/skills --list
npx skills add you/skills --skill pr-review -g
```

It finds every `<group>/<skill>/SKILL.md` and ignores `sets/` and `lessons/`. If the vault is a public GitHub repo, the skills appear on [skills.sh](./skills-sh) once people install them this way.
