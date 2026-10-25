package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
)

// printHelp prints the command list, or cmd's help, to standard output:
// it is what was asked for.
func (c *cli) printHelp(cmd *Command) {
	var b strings.Builder
	if cmd == nil {
		b.WriteString("skl: your agent skills, in one repo, on every machine.\n\n")
		b.WriteString("Usage\n  skl <command> [arguments] [flags]\n\nCommands\n")
		rows := make([][2]string, len(c.cmds))
		for i, cmd := range c.cmds {
			rows[i] = [2]string{cmd.Name, cmd.Summary}
		}
		table(&b, rows)
		b.WriteString("\nFlags\n")
		flagTable(&b, append([]Flag{helpFlag}, globalFlags...))
		b.WriteString("\nRun skl help <command> for its arguments, flags and an example.\n")
	} else {
		help := cmd.Help
		if help == "" {
			help = cmd.Summary
		}
		fmt.Fprintf(&b, "%s\n\nUsage\n  %s\n\nFlags\n", help, strings.TrimSpace("skl "+cmd.Name+" "+cmd.Args))
		flagTable(&b, append([]Flag{helpFlag}, cmd.Flags...))
		if cmd.Example != "" {
			fmt.Fprintf(&b, "\nExample\n%s\n", indent(cmd.Example))
		}
	}
	fmt.Fprint(c.st.Stdout, b.String())
}

func flagTable(b *strings.Builder, flags []Flag) {
	rows := make([][2]string, len(flags))
	for i, f := range flags {
		name := "--" + f.Name
		switch {
		case f.Name == "help":
			name += ", -h"
		case f.Value != "":
			name += " <" + f.Value + ">"
		}
		rows[i] = [2]string{name, f.Usage}
	}
	table(b, rows)
}

func table(b *strings.Builder, rows [][2]string) {
	w := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintf(w, "  %s\t%s\n", r[0], r[1])
	}
	_ = w.Flush() // writes to a strings.Builder never fail
}
