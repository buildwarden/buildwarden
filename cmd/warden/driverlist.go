package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver"
)

var flagDriverListJSON bool

var driverCmd = &cobra.Command{
	Use:   "driver",
	Short: "Show the drivers warden can find",
}

var driverListCmd = &cobra.Command{
	Use:   "list",
	Args:  cobra.NoArgs,
	Short: "List the drivers warden can find, and which file runs for each",
	Long: `List one entry per driver name warden can find: the five bundled
drivers, and every warden-driver-<name> on PATH or beside warden. Each entry
gives the file that would run and where it was found (PATH, beside-warden,
linked, or path when --driver or WARDEN_DRIVER names the file), and beneath it
every other copy in lookup order: the ones an earlier copy shadows, and the
ones that cannot be run (skipped). * marks the driver this invocation's
--driver, WARDEN_DRIVER and config file select.

It reads directories only and runs no driver.`,
	RunE: runDriverList,
}

func init() {
	driverListCmd.Flags().BoolVar(&flagDriverListJSON, "json", false,
		"print one JSON object")
	driverCmd.AddCommand(driverListCmd)
	rootCmd.AddCommand(driverCmd)
}

func runDriverList(cmd *cobra.Command, _ []string) error {
	var notes []string
	sel := &selection{value: "container", source: sourceDflt}
	cfg, err := resolveConfig()
	if err != nil {
		notes = append(notes, "the config file was not read: "+err.Error())
		switch {
		case flagDriver != "":
			sel = &selection{value: flagDriver, source: sourceFlag}
		case os.Getenv(sourceEnv) != "":
			sel = &selection{value: os.Getenv(sourceEnv), source: sourceEnv}
		}
	} else {
		s := driverSelection(cfg)
		sel = &s
	}
	env := hostLookupEnv()
	list, selErr := listDrivers(env, sel)
	if selErr != nil {
		notes = append(notes, "the selected driver cannot be used: "+selErr.Error())
	} else if !isPathSelection(sel.value, env.windows) {
		if _, err := findDriver(*sel, env); err != nil {
			notes = append(notes, "the selected driver cannot be used: "+err.Error())
		}
	}
	if flagDriverListJSON {
		return printDriverListJSON(cmd.OutOrStdout(), list)
	}
	printDriverList(cmd.OutOrStdout(), list)
	for _, n := range notes {
		fmt.Fprintln(cmd.OutOrStdout(), driver.EscapeOutside(n))
	}
	return nil
}

func printDriverListJSON(w io.Writer, list []listedDriver) error {
	if list == nil {
		list = []listedDriver{}
	}
	data, err := json.Marshal(map[string]any{"drivers": list})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

// foundLabel is how the table's FOUND column says where a driver was found.
func foundLabel(d listedDriver) string {
	switch d.Found {
	case foundBeside:
		return "beside warden"
	case foundPath:
		return "path (" + d.Source + ")"
	case "":
		return "not found"
	}
	return d.Found
}

func printDriverList(w io.Writer, list []listedDriver) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\t\tRUNS\tFOUND")
	for _, d := range list {
		mark := ""
		if d.Selected {
			mark = "*"
		}
		runs := driver.EscapeOutside(d.Path)
		switch d.Found {
		case foundLinked:
			runs = "(linked into warden)"
		case "":
			runs = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", d.Name, mark, runs, foundLabel(d))
		for _, s := range d.Shadowed {
			if s.Found == foundLinked {
				fmt.Fprintln(tw, "\t\t  shadows the driver linked into warden\t")
				continue
			}
			where := ""
			if s.Found == foundBeside {
				where = " (beside warden)"
			}
			fmt.Fprintf(tw, "\t\t  shadows %s%s\t\n", driver.EscapeOutside(s.Path),
				where)
		}
		for _, s := range d.Skipped {
			fmt.Fprintf(tw, "\t\t  skips %s (%s)\t\n", driver.EscapeOutside(s.Path),
				s.Reason)
		}
	}
	_ = tw.Flush()
}

// extCommand reports whether args, warden's arguments, are the ext form
// `warden <name> <args...>` (DRV-11, DRV-29): a first argument that is not a
// flag and not a warden command, and names a driver warden can find. ok is
// false when the line is warden's own, or the name finds no driver, which is
// then cobra's unknown-command error as before.
func extCommand(args []string) (name string, rest []string, ok bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", nil, false
	}
	name = args[0]
	for _, c := range rootCmd.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return "", nil, false
		}
	}
	if contains(wardenCommands, name) || strings.HasPrefix(name, "__") ||
		checkDriverName(name) != nil {
		return "", nil, false
	}
	fd, err := findByName(name, hostLookupEnv())
	if err != nil && fd != nil {
		return "", nil, false
	}
	return name, args[1:], true
}

// runExt runs `warden <name> <args...>` as the driver's ext <args...>, with
// the arguments as typed, and returns warden's exit code. Its config file is
// the one WARDEN_CONFIG names, else ./warden.toml, named without being
// parsed: no flag of warden's stands before <name>.
func runExt(name string, rest []string) error {
	env := hostLookupEnv()
	fd, err := findByName(name, env)
	if err != nil {
		return err
	}
	if fd.Linked {
		return driver.Fixable("see 'warden --help'",
			"unknown command %q for \"warden\": the driver %q linked into warden has "+
				"no commands of its own", name, name)
	}
	bin, err := describeDriver(fd, time.After)
	if err != nil {
		return err
	}
	if len(rest) == 1 && (rest[0] == "--help" || rest[0] == "-h") || len(rest) == 0 {
		fmt.Printf("The commands of the driver %q (warden %s <command>):\n", name, name)
		for _, e := range bin.desc.Ext {
			fmt.Printf("  %-24s %s\n", strings.Join(e.Path, " "), e.Usage)
		}
		if len(rest) == 0 && len(bin.desc.Ext) > 0 {
			return exitStatusError(1)
		}
		return nil
	}
	if len(bin.desc.Ext) == 0 || !bin.desc.lists("ext") {
		return driver.Fixable("", "the driver %q has no commands of its own", name)
	}
	_, err = bin.call("ext", withConfig(map[string]any{"contract": bin.contract},
		unparsedConfigFile()), callOpts{argv: rest})
	var es exitStatusError
	if errors.As(err, &es) {
		return es
	}
	return err
}
