# Commands

::: warning Design preview
Parts of this page aren't built yet. See [What works in v0.1](/design/roadmap#what-works-in-v0-1).
:::

Wherever a command takes `<selectors>`, `<repo>` or `<ref>`, any form from [Selectors and references](./repos) works.

## Start

| Command | Description |
|---|---|
| `skl init <repo> [selectors]` | Clone your vault, or set up a new one, and load `selectors` if given. Offers to import the skills in your [targets](./config)' folders (`~/.claude/skills` by default) that aren't in a vault, and with `--scope project` the repo's too; imported skills join the active skill set. See [Getting started](../guide/getting-started#make-your-vault). |
| `skl import <name>… [--group <group>]` | Copy skills skl didn't create into your vault; each original becomes the loaded skill and joins the active skill set, as `skl add` would. A skill a repo commits is refused: see [Skills the repo commits](../guide/skill-sets#skills-the-repo-commits). The group defaults to your vault's owner (`you` for `you/skills`). See [Add a skill you made](../guide/skills#add-a-skill-you-made). |

## Load

| Command | Description |
|---|---|
| `skl use <selectors>` | Replace the active skill set. See [selectors](./repos#selectors). |
| `skl add <selectors>` | Add to the active skill set. |
| `skl drop <selectors>` | Remove from the active skill set. |
| `skl save <name>` | Save the selectors of the active skill set, the user scope's or with `--scope project` the repo's, as a named skill set. Asks before overwriting one. See [Named skill sets](../guide/skill-sets#named-skill-sets). |

## See where you stand

| Command | Description |
|---|---|
| `skl status` | Show what's loaded, where each skill came from, your named skill sets, and everything that needs you: changes not saved back yet, skills not in your vault, selectors that match nothing, updates that `skl sync` found, and changes not pushed. Each such line ends with the command to run. Works offline. |
| `skl history` | List generations, newest first, with the repo each one ran in if it changed a project scope. |

Both take `--json` for [scripts](#json).

## From others

| Command | Description |
|---|---|
| `skl learn <peer> [skill] ["<text>"]` | Copy a skill or skill set you don't have, or with text build a new skill from it. Improve a skill you have with what its peer does better. With only your skill, take its author's changes: as they are if you never changed it, else by a lesson. With no arguments, take every available update. See [Learn](../guide/learn). |
| `skl open [ref]` | Open a skill, skill set or repo in your browser, or your vault without `ref`. Always prints the URL. See [Web pages](./repos#web-pages). |

## Undo

| Command | Description |
|---|---|
| `skl rollback [N]` | Go back one generation, or to generation `N`, in every scope those generations changed. See [Rollback](../guide/rollback). |

## Sync

| Command | Description |
|---|---|
| `skl sync` | Commit hand edits, pull, push, reload what's loaded, and check the skills you learned for updates. On a conflict, resolve with Claude, by hand or abort; `--continue` and `--abort` finish or undo a merge. See [Sync](../guide/sync). |

Optional [third-party parts](../thirdparty/) add more, such as `skl search`.

## Help

| Command | Description |
|---|---|
| `skl help [command]` | List the commands, or show one command's arguments, flags and an example. `skl <command> --help` and `-h` do the same. `skl` alone lists the commands. |
| `skl --version` | Print skl's version and the commit it was built from: `skl 0.1.0 (b0a0fb2)`. |
| `skl completion <shell>` | Print a completion script for `bash`, `zsh` or `fish`. It completes commands, flags, and the groups, skills and `@sets` in your vault, offline. |

To turn completion on, add one line to your shell's startup file:

```sh
source <(skl completion bash)   # ~/.bashrc
source <(skl completion zsh)    # ~/.zshrc
skl completion fish | source    # ~/.config/fish/config.fish
```

A mistyped command gets a suggestion and exits with code 2:

```
✗ unknown command stauts · skl status
```

## Flags

Flags start with `--` and can go anywhere on the line, as `--group writing` or `--group=writing`. Every other word is an argument, so `-review/test-gaps` is a selector, not a flag. `--` ends the flags: every word after it is an argument.

The one exception is `-h`, which always means `--help`. So a group can't be named `h`.

| Flag | Commands | Effect |
|---|---|---|
| `--help`, `-h` | all | Show the command's help, and change nothing. |
| `--scope user\|project` | `init` `import` `use` `add` `drop` `save` `learn` | Where to load skills, or for `import`, where to read them. `user` is `~/.claude/skills`; `project` is `.claude/skills` at the root of the git repo you're in. The `agents` [target](./config) uses `~/.agents/skills` and `.agents/skills`. Outside a git repo, `project` is an error. Defaults to `scope` in the [config](./config), else `user`. See [For one repo only](../guide/skill-sets#for-one-repo-only). |
| `--dry-run` | every command that changes something | Print what would change, and change nothing. See [below](#dry-run). |
| `--yes` | every command that asks | Take the first answer to every question. See [Questions](#questions). |
| `--json` | `status` `history` | Print [JSON](#json) instead of text. |
| `--group <group>` | `import` | The group to import into. |
| `--continue`, `--abort` | `sync` | Finish or undo a merge you resolved by hand. |

`skl status` shows your user scope and, inside a git repo, that repo's project scope.

### Dry run

With `--dry-run`, a command prints its plan, the `→` line and the `+ ~ −` lines, then stops with exit code 0. Nothing is written, cloned, created or committed, and no question is asked.

A real run first saves your changes to loaded skills and commits your hand edits in the vault (see [Your skills](../guide/skills)), so the plan starts with that, then shows the command's own plan:

```
→ save pr-review → review/pr-review
→ commit sets/backend.toml
→ overwrite sets/backend.toml
+ data
```

| Command | What `--dry-run` shows |
|---|---|
| `init` | Whether it would clone or create the repo (`→ clone you/skills` or `→ new vault you/skills`), what it would import, and what it would load. |
| `import`, `use`, `add`, `drop`, `save`, `rollback` | The skills it would load, replace or unload, and the files it would write. |
| `learn <peer> …` | Whether it would copy, update or run a lesson, and into which skill. The engine doesn't run. |
| `learn` | Every available update, and how each would be taken. |
| `sync` | What it would commit, pull, push and reload. It fetches the remote but doesn't merge or push. |
| `open` | The URL, without starting a browser. |

`status`, `history` and `help` change nothing, so `--dry-run` makes no difference to them. With `sync --continue` or `--abort`, it's a usage error.

## Questions

A question shows its answers; the first one is the default, taken with Enter. skl asks only when its input is a terminal.

`--yes` takes the first answer to every question without showing it. Each first answer is the one you'd usually pick, and every change it makes can be [rolled back](../guide/rollback):

| Question | First answer |
|---|---|
| `skl init`: import the skills it found? | yes, all of them |
| `skl learn`: add someone else's skill or scripts? | yes |
| `skl learn`: a license that may not fit your vault | yes, keep their license and notice |
| `skl save`: overwrite a named skill set? | yes |
| `skl learn`: land the lesson? | replace |

One question puts `no` first: `skl save` overwriting a named skill set with one that would remove every line. Enter keeps the file; `--yes` still overwrites it. See [Named skill sets](../guide/skill-sets#named-skill-sets).

Without a terminal and without `--yes`, a question stops the command before it changes anything:

```
✗ Add it? needs an answer · skl learn someone/agent-skills/pdf --yes
```

Two things always need a terminal, because the [engine](../guide/learn#who-does-the-thinking) asks you questions in them: a [lesson](../guide/learn#how-a-lesson-works) and a [sync conflict resolved with Claude](../guide/sync#with-claude). `--yes` doesn't answer them. Without a terminal, `skl learn` stops before the engine runs. `skl sync` undoes the merge, keeps your local commits, and stops:

```
✗ conflict in sets/backend.toml needs you · skl sync, in a terminal
```

## Output

Every command uses the same marks:

| Mark | Means | Example |
|---|---|---|
| `→` | What skl is about to do | `→ copy into someone/pdf` |
| `◇` | A step it did | `◇ saved      pr-review → review/pr-review` |
| `+` `~` `−` | A skill loaded, replaced or updated, unloaded | `+ pdf` |
| `?` `›` | A question; the first answer is the default | `? Add it?   › yes   no` |
| `!` | A warning, with the command that deals with it | `! 2 changes not pushed · skl sync` |
| `✗` | An error, with the command to run | `✗ someone/agent-skills/pdf is someone else's skill · skl learn someone/agent-skills/pdf` |
| `✓` | The result, last; for a change, its generation | `✓ generation 14 · skl rollback to undo` |

A command that doesn't fit on the line moves to the next one, after `·`; one that fits stays on the line.

A command that changes nothing makes no generation and ends with `✓ nothing to change`. Running it again is always safe. A changed active skill set counts as a change, even if no skill folder changes: see [Loading pipeline](../design/architecture#loading-pipeline).

**Where output goes.** What you asked for goes to standard output: the report of `skl status`, the list of `skl history`, the URL of `skl open`, a `--dry-run` plan, and `--json`. Everything else, the marks of a running command and its questions, goes to standard error. So `skl open review/pr-review | pbcopy` copies only the URL, and `skl status > status.txt` saves the report.

**Color.** skl colors its marks only when the output is a terminal. Setting `NO_COLOR` to a non-empty value or `TERM=dumb` turns color off. The marks themselves stay the same. With `TERM=dumb`, a question is a plain line, `? Add it?  yes / no:`: type an answer, or press Enter for the first one.

### JSON

`skl status --json` and `skl history --json` print one JSON object to standard output, with no marks, no color and no questions. The format may change until skl 1.0.

```json
{
  "generation": 14,
  "scopes": [
    {
      "scope": "user",
      "paths": ["/home/you/.claude/skills"],
      "selectors": ["@backend"],
      "skills": [
        { "name": "pr-review", "skill": "you/pr-review" },
        { "name": "pdf", "skill": "someone/pdf" }
      ]
    }
  ],
  "sets": ["backend", "base"],
  "needs": [
    {
      "skill": "you/pr-review",
      "state": "changed",
      "command": "skl sync"
    },
    {
      "skill": "someone/pdf",
      "state": "update available",
      "command": "skl learn someone/pdf"
    },
    {
      "name": "my-new-skill",
      "state": "not in your vault",
      "command": "skl import my-new-skill"
    },
    { "state": "2 changes not pushed", "command": "skl sync" }
  ]
}
```

`needs` holds the lines of `skl status` that need you, each with the command that deals with it.

```json
{
  "generations": [
    {
      "generation": 12,
      "time": "2026-10-22T14:03:00Z",
      "command": "learn deep-pr-review → review/pr-review",
      "commit": "9b1d4e0c2f7a8e35d6b1c0f4a9e2d7b3c5f8a1e6"
    }
  ]
}
```

To see the git and engine commands skl runs, set `SKL_DEBUG=1`: each one is printed to standard error before it runs. skl's [internal diagnostics](./config#environment-variables) are printed there too.

## Exit codes

| Code | Means |
|---|---|
| `0` | Done. Also when there were warnings (`!`), when there was nothing to change, and after `--dry-run`. |
| `1` | An error. The `✗` line says what went wrong and what to run. This includes a question that couldn't be asked and a sync that stopped at a conflict. |
| `2` | A usage error: an unknown command or flag, a missing argument, or a selector or reference that can't be parsed. Nothing changed. |
| `130` | Interrupted with Ctrl-C. |

Ctrl-C at a question takes its last answer (no, discard or abort), and skl exits with `130`. Ctrl-C while skl is changing things lets it finish or undo the step it's in, so your loaded skills and your vault are never left half changed. A second Ctrl-C stops at once.

## Errors

The errors any command can meet:

```
✗ no vault on this machine · skl init <repo>
```

Every command except `init`, `help`, `completion` and `open` with someone else's repo needs your vault.

```
✗ another skl is running (pid 4182) · skl sync, once it's done
```

One skl runs at a time on a machine, so two commands never change your vault or your loaded skills at once.

```
✗ a sync is in progress · skl sync --continue
```

While a merge you resolve [by hand](../guide/sync#by-hand) is open, only `status`, `history`, `open`, `help` and `sync --continue` or `--abort` run.

```
✗ can't reach github.com (Could not resolve host) · skl sync
```

When the network or the remote fails, skl shows git's error and stops, and the hint is the command you ran: a host that `skl init you/skills` can't resolve, or that refuses or times out, ends `· skl init you/skills`. `skl sync` keeps what it did locally first: your changes are saved and committed, and `skl status` lists them as not pushed. `skl learn` and `skl init` change nothing. Access problems fail the same way, without asking for a password: see [Authentication](./repos#authentication). `skl init` has [its own line](../guide/getting-started#make-your-vault) for a missing repo or denied access.
