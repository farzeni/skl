package cli

import "context"

// commands is the command table. A command appears here once it is built,
// with only the flags it accepts, so help never lists what doesn't work.
func (c *cli) commands() []*Command {
	return []*Command{
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
