# Other people's skills

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Use skills from any git repo after seeing what they do, make one your own, and keep up with its author.

## Find one

Any git repo with skill folders in it works, on any host. To look skills up by name, the optional [skills.sh part](../thirdparty/skills-sh) adds `skl search`.

## Try one

```sh
skl add someone/agent-skills/deep-pr-review
```

loads the skill next to the ones you have. skl fetches only that folder and pins it to the commit it fetched. The first time, it shows what the skill does and which scripts it runs, and asks before loading it.

The skill stays in its author's repo, not in your vault. skl keeps using the pinned commit until you run [`skl upstream`](#get-the-author-s-changes). To pin a tag or a commit yourself, or to load from another host, see [Selectors and references](../reference/repos).

## Use, copy or learn?

| You want | Run | What you get |
|---|---|---|
| The skill as its author keeps it | `skl add someone/agent-skills/deep-pr-review` | A read-only skill, pinned to a commit |
| The whole skill, to change it | `skl learn someone/agent-skills/deep-pr-review` | A copy in your vault that records its source |
| One thing it does well, in a skill you have | `skl learn someone/agent-skills/deep-pr-review review/pr-review "its security checks"` | A [lesson](./learn): Claude adds it to your skill |

## Make it yours

`skl learn` with a skill you don't have, and no text, copies it into your vault as it is and records where it came from in `.skl-origin.toml`. The copy is yours to edit. This needs no Claude.

```sh
skl learn anthropics/skills/pdf
skl learn anthropics/skills/pdf anthropics-pdf
```

The first copies `pdf` into `anthropics/pdf`, a group named after the repo's owner, if you have no `pdf` skill. The second copies it as `anthropics-pdf`. A copy under a new name also gets that name in its `SKILL.md`, so it doesn't clash with the original. If you already have the skill, the same commands run a lesson instead: see [Which skill it changes](./learn#which-skill-it-changes).

## Copy a skill set

`skl learn` also copies a [skill set](./skill-sets) you don't have:

```sh
skl learn someone/skills/@backend
```

saves it as `sets/backend.toml`, with each selector rewritten to point at their repo, so `review` becomes `someone/skills/review`. Their skills stay in their repo. Add a second argument such as `@their-backend` to save it under another name. A skill set is never rewritten by Claude: if you already have `@backend`, take their changes with `skl upstream @backend --merge`.

## Get the author's changes

`skl upstream` lists your skills whose source or peer has new commits:

```
anthropics/pdf    anthropics/skills     4 behind  you changed 2 files
review/pr-review  someone/agent-skills  1 behind
```

To take only some of the changes, with Claude telling the author's changes from yours, [learn again](./learn#when-the-peer-changes):

```sh
skl learn anthropics/pdf "only the faster table extraction"
```

### Take everything

For a copy, you can also take all of the author's changes without Claude:

```sh
skl upstream anthropics/pdf --merge
```

skl merges three versions of each file: the commit you copied, your copy, and the author's latest. If both sides changed the same lines, the file gets standard conflict markers. Edit it, then finish with:

```sh
skl upstream --continue
```

A skill that Claude built from text isn't a copy, so `--merge` stops and tells you to run `skl learn` on it instead.

## You changed someone's skill

A skill you loaded from someone else's repo is a copy too, and skl never writes your changes into their repo or its cache. If you change one, skl stops before it refreshes or reloads that skill, and asks what to do:

```
? pdf changed since it was copied from anthropics/skills @3f9a1c2
  › save it to my vault   discard my changes   keep for now
```

- **Save it to my vault** copies the skill as of the pinned commit into your vault, the way [Make it yours](#make-it-yours) does, then adds your changes. Its source is recorded, so `skl upstream` keeps working. If your vault already has a skill with that name, skl asks for another one.
- **Discard** puts the pinned version back.
- **Keep for now** leaves your copy as it is. skl doesn't refresh it, and `skl status` flags it. Outside a terminal, this is what happens.
