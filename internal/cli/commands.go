package cli

import (
	"context"
	"time"

	"github.com/farzeni/skl/internal/app"
)

// dryRun is the --dry-run flag, the same for every command that changes
// something.
var dryRun = Flag{Name: "dry-run", Usage: "Print what would change, and change nothing"}

// asJSON is the --json flag of the commands that print a report.
var asJSON = Flag{Name: "json", Usage: "Print JSON instead of text"}

// yes is the --yes flag, the same for every command that asks.
var yes = Flag{Name: "yes", Usage: "Take the first answer to every question"}

// app builds the commands' dependencies. Only commands that use them read the
// config, so a bad config file doesn't get in the way of help.
func (c *cli) app(ctx context.Context) (*app.App, error) {
	return app.New(ctx, app.Deps{UI: c.ui, Getenv: c.getenv, Log: c.log, Now: time.Now})
}

// commands is the command table. A command appears here once it is built,
// with only the flags it accepts, so help never lists what doesn't work.
func (c *cli) commands() []*Command {
	return []*Command{
		{
			Name:    "init",
			Args:    "<repo>",
			Summary: "Clone your vault, or set up a new one",
			Help: "Clone your vault from <repo>, or set up a new one if <repo> is empty.\n" +
				"skl never creates a repo: make an empty one on your git host first.\n" +
				"<repo> is owner/repo, host/path, a URL or a path.",
			Example: "skl init you/skills",
			Flags:   []Flag{dryRun},
			MinArgs: 1,
			MaxArgs: 1,
			Run: func(ctx context.Context, in Input) error {
				a, err := c.app(ctx)
				if err != nil {
					return err
				}
				return a.Init(ctx, in.Args[0], in.Has("dry-run"))
			},
		},
		c.loadCommand("use", "Replace the active skill set",
			"Load exactly the skills these selectors name, and unload the rest.", "skl use review someone/pdf -review/test-gaps",
			(*app.App).Use),
		c.loadCommand("add", "Add to the active skill set",
			"Load these selectors too, after the ones already active.", "skl add review/pr-review",
			(*app.App).Add),
		c.loadCommand("drop", "Remove from the active skill set",
			"Unload what these selectors name.", "skl drop review/test-gaps",
			(*app.App).Drop),
		{
			Name:    "save",
			Args:    "<name>",
			Summary: "Save the active skill set as a named skill set",
			Help: "Save the selectors of the active skill set as sets/<name>.toml in your vault,\n" +
				"to load anywhere with @<name>. Asks before overwriting one.",
			Example: "skl save backend",
			Flags:   []Flag{dryRun, yes},
			MinArgs: 1,
			MaxArgs: 1,
			Run: func(ctx context.Context, in Input) error {
				a, err := c.app(ctx)
				if err != nil {
					return err
				}
				return a.Save(ctx, in.Args[0], in.Has("dry-run"))
			},
		},
		{
			Name:    "status",
			Summary: "Show what's loaded and what needs you",
			Help: "Show what's loaded and from which vault skill, your named skill sets, and\n" +
				"everything that needs you, each with the command to run. Works offline.",
			Example: "skl status --json",
			Flags:   []Flag{asJSON, dryRun},
			Run: func(ctx context.Context, in Input) error {
				a, err := c.app(ctx)
				if err != nil {
					return err
				}
				return a.Status(ctx, in.Has("json"))
			},
		},
		{
			Name:    "history",
			Summary: "List generations, newest first",
			Help: "List every generation on this machine, newest first: its number, how long\n" +
				"ago it was made, and the command that made it. Works offline.",
			Example: "skl history --json",
			Flags:   []Flag{asJSON, dryRun},
			Run: func(ctx context.Context, in Input) error {
				a, err := c.app(ctx)
				if err != nil {
					return err
				}
				return a.History(ctx, in.Has("json"))
			},
		},
		{
			Name:    "rollback",
			Args:    "[N]",
			Summary: "Go back one generation, or to generation N",
			Help: "Load again what was loaded at generation N, one before the newest if left\n" +
				"out, and put back the skills that changed since in revert commits. It never\n" +
				"rewrites history, and is a generation itself: skl rollback undoes it.",
			Example: "skl rollback 10",
			Flags:   []Flag{dryRun},
			MaxArgs: 1,
			Run: func(ctx context.Context, in Input) error {
				a, err := c.app(ctx)
				if err != nil {
					return err
				}
				return a.Rollback(ctx, in.Args, in.Has("dry-run"))
			},
		},
		{
			Name:    "help",
			Args:    "[command]",
			Summary: "List the commands, or show one command's help",
			Help:    "List the commands, or show one command's arguments, flags and an example.\nskl <command> --help and -h do the same.",
			Example: "skl help help",
			MaxArgs: 1,
			Run: func(_ context.Context, in Input) error {
				if len(in.Args) == 0 {
					c.printHelp(nil)
					return nil
				}
				cmd := c.lookup(in.Args[0])
				if cmd == nil {
					return c.unknownCommand(in.Args[0], "skl help ")
				}
				c.printHelp(cmd)
				return nil
			},
		},
	}
}

// loadCommand is use, add or drop: they take the same words and the same flags.
func (c *cli) loadCommand(name, summary, help, example string, run func(*app.App, context.Context, []string, bool) error) *Command {
	return &Command{
		Name:    name,
		Args:    "<selectors>",
		Summary: summary,
		Help: help + "\n" +
			"A selector is a group, group/skill or @set from your vault; -selector removes.\n" +
			"A skill from someone else's repo gets in with skl learn first.",
		Example: example,
		Flags:   []Flag{dryRun},
		MinArgs: 1,
		MaxArgs: -1,
		Run: func(ctx context.Context, in Input) error {
			a, err := c.app(ctx)
			if err != nil {
				return err
			}
			return run(a, ctx, in.Args, in.Has("dry-run"))
		},
	}
}
