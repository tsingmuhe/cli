package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"text/template"
)

// Command contains everything needed to run an application that
// accepts a string slice of arguments such as os.Args[1:]. A given
// Command may contain sub-commands in Commands.
type Command struct {
	// UsageLine is the one-line usage message.
	// All the words in the usage line before a flag or argument are taken to be the command name.
	UsageLine string

	// Short is the short description shown in the 'help' output.
	Short string

	// Long is the long message shown in the 'help' output.
	Long string

	// Run runs the command.
	// The args are the arguments after the command name.
	//
	// Note: Commands and Run are usually mutually exclusive.
	Run func(ctx context.Context, cmd *Command, args []string) error

	// Commands lists the available commands.
	// The order here is the order in which they are printed by 'help'.
	//
	// Note: Commands and Run are usually mutually exclusive.
	Commands []*Command
}

// LongName returns the command's long name: all the words in the usage line before a flag or argument.
//
// Note: the command's long name must not contain any of the following characters: ()<>[]|-
func (c *Command) LongName() string {
	name := c.UsageLine
	if i := strings.IndexAny(name, "[]<>()|-"); i >= 0 {
		name = name[:i]
	}

	if i := strings.LastIndex(name, " "); i >= 0 {
		name = name[:i]
	}

	return name
}

// Name returns the command's short name: the last word in the usage line before a flag or argument.
func (c *Command) Name() string {
	name := c.LongName()
	if i := strings.LastIndex(name, " "); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// ProgramName returns the base name of the executable program,
// which is the first word in the usage line.
func (c *Command) ProgramName() string {
	name := c.LongName()
	if i := strings.Index(name, " "); i >= 0 {
		name = name[:i]
	}
	return name
}

// HelpName returns the command line string for invoking help on this command,
// by inserting "help" after the program name.
func (c *Command) HelpName() string {
	name := c.LongName()
	if i := strings.Index(name, " "); i >= 0 {
		name = name[:i] + " help" + name[i:]
	} else {
		name = name + " help"
	}
	return name
}

// Runnable reports whether the command can be run.
func (c *Command) Runnable() bool {
	return c.Run != nil
}

// Lookup returns the subcommand with the given name, if any.
// Otherwise it returns nil.
//
// Lookup ignores subcommands that have len(c.Commands) == 0 and c.Run == nil.
// Such subcommands are only for use as arguments to "help".
func (c *Command) Lookup(name string) *Command {
	for _, sub := range c.Commands {
		if sub.Name() == name && (len(sub.Commands) > 0 || sub.Runnable()) {
			return sub
		}
	}
	return nil
}

// Usage puts out the usage for the command.
// Used when a user provides invalid input.
func (c *Command) Usage() {
	fmt.Fprintf(os.Stderr, "Usage:  %s\n", c.UsageLine)
	fmt.Fprintf(os.Stderr, "Run '%s' for details.\n", c.HelpName())
	SetExitStatus(2)
}

// Run runs the command.
func Run(cmd *Command, args []string) int {
	run(cmd, args, func(args []string) {
		if HelpFunc == nil {
			defaultHelpFunc(cmd, args)
		} else {
			HelpFunc(cmd, args)
		}
	})

	return GetExitStatus()
}

func run(cmd *Command, args []string, helpFunc func(args []string)) {
	if len(args) > 0 && args[0] == "help" { // 'proc help'
		helpFunc(args[1:])
		return
	}

	cmd, used := lookupCmd(cmd, args)
	if len(cmd.Commands) > 0 {
		if used >= len(args) { //  'proc' and 'proc foo'
			helpFunc(args)
			return
		}

		if args[used] == "help" {
			// 'proc help foo'
			// 'proc foo help'
			// 'proc help foo bar'
			// 'proc foo help bar'
			helpFunc(append(slices.Clip(args[:used]), args[used+1:]...))
			return
		}

		// 'proc -h/--help'
		if used == 0 && (args[used] == "-h" || args[used] == "--help") {
			helpFunc(nil)
			return
		}

		fmt.Fprintf(os.Stderr, "%s %s: unknown command.\n", cmd.ProgramName(), strings.Join(args, " "))
		fmt.Fprintf(os.Stderr, "Run '%s' for details.\n", cmd.HelpName())
		SetExitStatus(2)
		return
	}

	invoke(cmd, args[used:])
}

// lookupCmd interprets the initial elements of args
// to find a command to run (cmd.Runnable() == true)
// or else a command group that ran out of arguments
// or had an unknown subcommand (len(cmd.Commands) > 0).
// It returns that command and the number of elements of args
// that it took to arrive at that command.
func lookupCmd(cmd *Command, args []string) (*Command, int) {
	used := 0

	for used < len(args) {
		c := cmd.Lookup(args[used])
		if c == nil {
			break
		}

		if c.Runnable() {
			cmd = c
			used++
			break
		}

		if len(c.Commands) > 0 {
			cmd = c
			used++
			if used >= len(args) || args[0] == "help" {
				break
			}
			continue
		}

		// len(c.Commands) == 0 && !c.Runnable() => help text; stop at "help"
		break
	}

	return cmd, used
}

func invoke(cmd *Command, args []string) {
	for _, arg := range args {
		if arg == "--" {
			break
		}

		// 'proc -h/--help'
		// 'proc foo bar -h/--help'
		if arg == "-h" || arg == "--help" {
			fmt.Fprintf(os.Stdout, "Usage:  %s\n", cmd.UsageLine)
			fmt.Fprintf(os.Stdout, "Run '%s' for details.\n", cmd.HelpName())
			return
		}
	}

	if err := cmd.Run(context.Background(), cmd, args); err != nil {
		SetExitStatus(1)
		return
	}
}

// HelpFunc is an optional function that generates the help message for a command.
// If not set, defaultHelpFunc will be used.
var HelpFunc func(*Command, []string)

func defaultHelpFunc(cmd *Command, args []string) {
Args:
	for _, arg := range args {
		for _, sub := range cmd.Commands {
			if sub.Name() == arg {
				cmd = sub
				continue Args
			}
		}

		fmt.Fprintf(os.Stderr, "%s help %s: unknown help topic.\n", cmd.ProgramName(), strings.Join(args, " "))
		fmt.Fprintf(os.Stderr, "Run '%s' for details.\n", cmd.HelpName())
		SetExitStatus(2)
		return
	}

	bw := bufio.NewWriter(os.Stdout)
	tmpl(bw, helpTemplate, cmd)
	bw.Flush()
}

var helpTemplate = `{{if not (len .Commands)}}{{if .Runnable}}Usage:  {{.UsageLine}}{{end}}{{if ne (len .Long) 0}}

{{.Long | trim}}{{end}}{{else}}{{if ne (len .Long) 0}}{{.Long | trim}}

{{end}}Usage:  {{.UsageLine}} <command> [arguments]

The commands are:
{{range .Commands}}{{if or (.Runnable) .Commands}}
	{{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}
{{end}}
`

// tmpl executes the given template text on data, writing the result to w.
func tmpl(w io.Writer, text string, data any) {
	t := template.New("top")
	t.Funcs(template.FuncMap{
		"trim": strings.TrimSpace,
	})

	template.Must(t.Parse(text))
	err := t.Execute(w, data)
	if err != nil {
		panic(err)
	}
}

var exitStatus = 0
var exitMu sync.Mutex

func SetExitStatus(n int) {
	exitMu.Lock()
	if exitStatus < n {
		exitStatus = n
	}
	exitMu.Unlock()
}

func GetExitStatus() int {
	return exitStatus
}
