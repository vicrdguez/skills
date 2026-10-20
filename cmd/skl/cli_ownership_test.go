package main

import (
	"bytes"
	"flag"
	"testing"

	"github.com/urfave/cli/v2"
)

// urfave/cli v2 shares mutable built-in flags across apps. Install immutable
// definitions before m.Run; each parse gets its own BoolFlag state. Application
// flags still use their ordinary implementations. These two built-ins have no
// environment, destination or action state to retain after parsing.
type testBuiltinFlag struct{ cli.BoolFlag }

func (f *testBuiltinFlag) Apply(set *flag.FlagSet) error {
	local := f.BoolFlag
	return local.Apply(set)
}

var testHelpCommand cli.Command

func prepareCLIIsolation() {
	template := cli.NewApp()
	template.Setup()
	testHelpCommand = *template.Command("help")
	testHelpCommand.HelpName = ""
	cli.HelpFlag = &testBuiltinFlag{*cli.HelpFlag.(*cli.BoolFlag)}
	cli.VersionFlag = &testBuiltinFlag{*cli.VersionFlag.(*cli.BoolFlag)}
}

// The library also inserts one shared help Command and mutates its metadata
// during setup. Private apps own a help Command at every level, keeping the
// library's original action and presentation instead of hiding help.
func privateCLIHelp(app *stageApp) {
	newHelp := func() *cli.Command {
		help := testHelpCommand
		return &help
	}
	var visit func(*cli.Command)
	visit = func(command *cli.Command) {
		// Preinstalling help must not turn a leaf into a subcommand-help page.
		if len(command.Subcommands) == 0 && command.CustomHelpTemplate == "" {
			command.CustomHelpTemplate = cli.CommandHelpTemplate
		}
		for _, child := range command.Subcommands {
			visit(child)
		}
		command.Subcommands = append(command.Subcommands, newHelp())
	}
	for _, command := range app.Commands {
		visit(command)
	}
	app.Commands = append(app.Commands, newHelp())
	app.Flags = append(app.Flags, cli.HelpFlag)
}

func TestPrivateCLINormalHelpMatchesProduction(t *testing.T) {
	for _, args := range [][]string{
		{"skl", "--help"}, {"skl", "--version"},
		{"skl", "implement", "--help"}, {"skl", "implement", "next", "--help"},
		{"skl", "help", "implement"}, {"skl", "implement", "help", "next"},
	} {
		var ordinary, private bytes.Buffer
		factory := deliveryForgeFactory(newForgeServer(t))
		production := newApp(factory, bytes.NewReader(nil), &ordinary, &ordinary)
		isolated := newApp(factory, bytes.NewReader(nil), &private, &private)
		privateCLIHelp(isolated)
		productionErr, privateErr := production.Run(args), isolated.Run(args)
		if productionErr != nil || privateErr != nil || ordinary.String() != private.String() {
			t.Fatalf("%v changed help or errors:\nordinary: %v\n%s\nprivate: %v\n%s", args, productionErr, &ordinary, privateErr, &private)
		}
	}
}
