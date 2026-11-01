package cli

import (
	"context"
	"time"

	"github.com/farzeni/skl/internal/app"
)

// dryRun is the --dry-run flag, the same for every command that changes
// something.
var dryRun = Flag{Name: "dry-run", Usage: "Print what would change, and change nothing"}

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
