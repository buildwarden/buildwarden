package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/buildwarden/buildwarden/driver"
)

// A driver's flags are not warden's: warden learns them from the selected
// driver's describe, so it finds the driver before it parses the command line
// (the design's Configuration; v1.md, Finding the driver before the command
// line is parsed). It scans the raw arguments for the command and for
// --driver and --config, selects the driver, registers the flags it declares
// on the commands that take them, and after the parse hands the driver what
// was typed as args. A flag neither warden nor the driver declares is then
// an unknown flag to cobra.

// driverAnnotation marks a flag registered for the selected driver; its value
// is the driver's name.
const driverAnnotation = "warden-driver-flag"

// commandsFor are warden's commands for each contract command that takes
// flags.
func commandsFor(command string) []*cobra.Command {
	switch command {
	case "run":
		return []*cobra.Command{buildCmd, shellCmd}
	case "doctor":
		return []*cobra.Command{doctorCmd}
	case "setup":
		return []*cobra.Command{setupCmd}
	case "image-prepare":
		return []*cobra.Command{imagePrepareCmd}
	case "clean":
		return []*cobra.Command{cleanCmd}
	}
	return nil
}

// contractCommand is the contract command a warden command line runs, from
// its command words, or "" for one that takes no driver flag.
func contractCommand(words []string) string {
	if len(words) == 0 {
		return ""
	}
	switch words[0] {
	case "build", "shell":
		return "run"
	case "doctor", "setup", "clean":
		return words[0]
	case "image":
		if len(words) > 1 && words[1] == "prepare" {
			return "image-prepare"
		}
	}
	return ""
}

// driverFlagState is what this invocation's scan found and registered.
type driverFlagState struct {
	// driverGiven and driverValue are --driver as the scan found it; the same
	// for --config.
	driverGiven, configGiven bool
	driverValue, configValue string
	// command is the contract command the line runs, or "".
	command string
	// sel is the driver the scan selected, and key the selection it made;
	// err why none could be selected, for help and for a flag error.
	sel *selectedDriver
	key selection
	err error
	// flags are the selected driver's flags, as its describe declares them.
	flags []describedFlag
	// args are the driver's flags as typed, one --name=value per occurrence
	// of a flag with a value; switches are the switches' last values.
	args     []string
	switches map[string]bool
}

// driverFlags is this invocation's state; execute makes it anew.
var driverFlags = &driverFlagState{}

// wardenFlagArity reports whether name is one of warden's own persistent
// flags, and whether it takes a value.
func wardenFlagArity(name string) (known, value bool) {
	switch name {
	case "driver", "config", "color":
		return true, true
	case "verbose", "v", "help", "h", "version":
		return true, false
	}
	return false, false
}

// scanValue finds --name's value in args, up to "--", in both spellings; the
// last occurrence wins. It knows no flag's arity but its own, as the
// contract's scan does.
func scanValue(args []string, name string) (string, bool) {
	var v string
	found := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if rest, ok := strings.CutPrefix(a, "--"+name+"="); ok {
			v, found = rest, true
		} else if a == "--"+name && i+1 < len(args) {
			v, found = args[i+1], true
			i++
		}
	}
	return v, found
}

// scanWords finds the command's words in args: the first argument that is
// not a flag, and for image the next one. arity tells a flag before them;
// the scan stops at one it does not know, which it returns.
func scanWords(args []string, arity func(string) (bool, bool)) (words []string,
	unknown string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return words, ""
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
			known, value := arity(name)
			if !known {
				return words, a
			}
			if value && !hasValue {
				i++
			}
			continue
		}
		words = append(words, a)
		if len(words) == 2 || words[0] != "image" {
			return words, ""
		}
	}
	return words, ""
}

// prepareDriverFlags scans args, selects the driver when the line runs a
// command that takes driver flags, and registers the flags it declares. An
// error is kept in driverFlags.err, for help and for a flag error: the
// command reports it itself when it selects the driver.
func prepareDriverFlags(args []string) {
	dropDriverFlags(rootCmd)
	st := &driverFlagState{switches: map[string]bool{}}
	driverFlags = st
	st.driverValue, st.driverGiven = scanValue(args, "driver")
	st.configValue, st.configGiven = scanValue(args, "config")
	words, unknown := scanWords(args, wardenFlagArity)
	if unknown != "" {
		// A flag warden does not know before the command can only be the
		// driver's; its arity is known once the driver is.
		if st.selectDriver() != nil {
			return
		}
		words, _ = scanWords(args, func(name string) (bool, bool) {
			if known, value := wardenFlagArity(name); known {
				return known, value
			}
			for _, f := range st.everyCommandFlags() {
				if f.Name == name {
					return true, f.Value
				}
			}
			return false, false
		})
	}
	if st.command = contractCommand(words); st.command == "" {
		return
	}
	if st.sel == nil && st.selectDriver() != nil {
		return
	}
	st.register()
}

// selectDriver selects the driver the scanned line names, from the config
// file the scan found, and reads its declared flags.
func (st *driverFlagState) selectDriver() error {
	path, source, err := configFileFrom(st.configGiven, st.configValue)
	var cfg *Config
	if err == nil {
		cfg, err = loadConfigFile(path, source)
	}
	if err != nil {
		st.err = err
		return err
	}
	if st.driverGiven {
		cfg.Runtime.Driver, cfg.Runtime.DriverSource = st.driverValue, sourceFlag
	}
	st.key = driverSelection(cfg)
	sd, err := findSelected(cfg)
	if err != nil {
		st.err = err
		return err
	}
	if st.flags, err = sd.declaredFlags(); err != nil {
		st.err = err
		return err
	}
	st.sel = sd
	return nil
}

// declaredFlags are the flags the driver's describe declares.
func (s *selectedDriver) declaredFlags() ([]describedFlag, error) {
	if s.bin != nil {
		return s.bin.desc.Flags, nil
	}
	d, err := s.linked.description(s.name())
	if err != nil {
		return nil, err
	}
	var out []describedFlag
	for _, f := range d.Flags {
		out = append(out, describedFlag{Name: f.Name, Value: f.Value,
			Commands: f.Commands, Usage: f.Usage})
	}
	return out, nil
}

// flaggedCommands are the contract commands that take flags.
var flaggedCommands = []string{"run", "doctor", "setup", "image-prepare", "clean"}

// everyCommandFlags are the driver's flags declared for every command it
// lists that takes flags, which are also accepted before the subcommand's
// name.
func (st *driverFlagState) everyCommandFlags() []describedFlag {
	var listed []string
	for _, c := range flaggedCommands {
		if st.sel != nil && st.sel.lists(c) {
			listed = append(listed, c)
		}
	}
	var out []describedFlag
	for _, f := range st.flags {
		every := len(listed) > 0
		for _, c := range listed {
			every = every && slices.Contains(f.Commands, c)
		}
		if every {
			out = append(out, f)
		}
	}
	return out
}

// register puts the driver's flags for the scanned command on warden's
// commands: one declared for every command it lists as a persistent flag,
// the others on the command's own flags.
func (st *driverFlagState) register() {
	every := st.everyCommandFlags()
	for _, f := range st.flags {
		if !slices.Contains(f.Commands, st.command) {
			continue
		}
		if slices.ContainsFunc(every, func(e describedFlag) bool {
			return e.Name == f.Name
		}) {
			st.add(rootCmd.PersistentFlags(), f)
			continue
		}
		for _, c := range commandsFor(st.command) {
			st.add(c.Flags(), f)
		}
	}
}

// add registers f in fs, recording each occurrence as the contract's args
// put it (DRV-62, DRV-63).
func (st *driverFlagState) add(fs *pflag.FlagSet, f describedFlag) {
	if fs.Lookup(f.Name) != nil {
		return
	}
	if f.Value {
		fs.Var(&valueFlag{st: st, name: f.Name}, f.Name, f.Usage)
	} else {
		fl := fs.VarPF(&switchFlag{st: st, name: f.Name}, f.Name, "", f.Usage)
		fl.NoOptDefVal = "true"
	}
	_ = fs.SetAnnotation(f.Name, driverAnnotation, []string{st.sel.name()})
}

// argsFor are the driver's args for command: each occurrence of a flag with
// a value in the order typed, then each switch that is on (DRV-62, DRV-63).
func (st *driverFlagState) argsFor(command string) []string {
	out := []string{}
	for _, a := range st.args {
		name, _, _ := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		if st.declares(command, name) {
			out = append(out, a)
		}
	}
	for _, f := range st.flags {
		if !f.Value && st.switches[f.Name] && slices.Contains(f.Commands, command) {
			out = append(out, "--"+f.Name)
		}
	}
	return out
}

func (st *driverFlagState) declares(command, name string) bool {
	return slices.ContainsFunc(st.flags, func(f describedFlag) bool {
		return f.Name == name && slices.Contains(f.Commands, command)
	})
}

// valueFlag is a driver's flag with a value: each Set is one occurrence.
type valueFlag struct {
	st   *driverFlagState
	name string
	last string
}

func (v *valueFlag) String() string { return v.last }
func (v *valueFlag) Type() string   { return "string" }
func (v *valueFlag) Set(s string) error {
	v.last = s
	v.st.args = append(v.st.args, "--"+v.name+"="+s)
	return nil
}

// switchFlag is a driver's switch: warden settles it, the last value
// winning, and sends --name only when it is on (DRV-63).
type switchFlag struct {
	st   *driverFlagState
	name string
}

func (s *switchFlag) String() string { return fmt.Sprint(s.st.switches[s.name]) }
func (s *switchFlag) Type() string   { return "bool" }
func (s *switchFlag) IsBoolFlag() bool {
	return true
}
func (s *switchFlag) Set(v string) error {
	switch v {
	case "true", "1", "t", "T", "TRUE", "True":
		s.st.switches[s.name] = true
	case "false", "0", "f", "F", "FALSE", "False":
		s.st.switches[s.name] = false
	default:
		return fmt.Errorf("want true or false, not %q", v)
	}
	return nil
}

// dropDriverFlags removes every flag a previous scan registered, from c and
// the commands under it, keeping warden's own: an invocation sees only its
// driver's flags, and a test runs many invocations in one process.
func dropDriverFlags(c *cobra.Command) {
	isDriver := func(f *pflag.Flag) bool { return f.Annotations[driverAnnotation] != nil }
	var local, persistent []*pflag.Flag
	found := false
	// A command's flags hold the persistent flags of its parents too, merged
	// in at its last parse: a driver's flag among them is dropped as well.
	c.Flags().VisitAll(func(f *pflag.Flag) { found = found || isDriver(f) })
	c.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
		if !isDriver(f) {
			local = append(local, f)
		}
	})
	c.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		found = found || isDriver(f)
		if !isDriver(f) {
			persistent = append(persistent, f)
		}
	})
	if found {
		c.ResetFlags()
		for _, f := range local {
			c.Flags().AddFlag(f)
		}
		for _, f := range persistent {
			c.PersistentFlags().AddFlag(f)
		}
	}
	for _, sub := range c.Commands() {
		dropDriverFlags(sub)
	}
}

// checkScan refuses a line whose parsed --driver or --config is not the one
// the scan found, which it would otherwise have selected the driver by while
// building with another (the design's Configuration, step 4).
func checkScan() error {
	st := driverFlags
	for _, c := range []struct {
		name         string
		given        bool
		scanned, got string
		changed      bool
	}{
		{"driver", st.driverGiven, st.driverValue, flagDriver, rootFlagChanged("driver")},
		{"config", st.configGiven, st.configValue, flagConfig, rootFlagChanged("config")},
	} {
		if c.given == c.changed && (!c.given || c.scanned == c.got) {
			continue
		}
		taker := ""
		for _, a := range st.args {
			name, v, _ := strings.Cut(strings.TrimPrefix(a, "--"), "=")
			if v == "--"+c.name || strings.HasPrefix(v, "--"+c.name+"=") {
				taker = name
			}
		}
		if taker == "" {
			return driver.Fixable("write --"+c.name+"=<value>",
				"--%s was not read as warden's --%s", c.name, c.name)
		}
		return driver.Fixable(fmt.Sprintf("give --%s its value, or write --%s=<value>",
			taker, taker), "--%s was read as the value of --%s", c.name, taker)
	}
	return nil
}

// rootFlagChanged reports whether warden's persistent flag name was given.
func rootFlagChanged(name string) bool {
	f := rootCmd.PersistentFlags().Lookup(name)
	return f != nil && f.Changed
}

// flagError is a parse error, or the reason the scan could not select the
// driver when that is why a flag is unknown.
func flagError(c *cobra.Command, err error) error {
	if driverFlags.err != nil && strings.Contains(err.Error(), "unknown flag") {
		return driverFlags.err
	}
	help := fmt.Sprintf("see '%s --help'", c.CommandPath())
	if name, ok := unknownFlagName(err); ok && driverFlags.sel != nil {
		d := driverFlags.sel.name()
		return driver.Fixable(fmt.Sprintf("see '%s --driver %s --help' for the flags "+
			"this driver takes", c.CommandPath(), d),
			"unknown flag --%s for driver %q", name, d)
	}
	return driver.Fixable(help, "%v", err)
}

// unknownFlagName is the flag pflag reports unknown, without its dashes.
func unknownFlagName(err error) (string, bool) {
	rest, ok := strings.CutPrefix(err.Error(), "unknown flag: --")
	if !ok {
		return "", false
	}
	return rest, true
}

// execute runs one command line as main does: the ext form, else the scan,
// then cobra.
func execute(args []string) error {
	if name, rest, ok := extCommand(args); ok {
		return runExt(name, rest)
	}
	prepareDriverFlags(args)
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

// usageTemplate is cobra's, with warden's flags and the selected driver's in
// sections of their own (Behaviour 1).
const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{with wardenFlags .LocalFlags}}

Flags:
{{.}}{{end}}{{with wardenFlags .InheritedFlags}}

Global Flags:
{{.}}{{end}}{{with driverFlagsHelp .}}

{{.}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

func init() {
	cobra.AddTemplateFunc("wardenFlags", func(fs *pflag.FlagSet) string {
		return flagUsages(fs, false)
	})
	cobra.AddTemplateFunc("driverFlagsHelp", driverFlagsHelp)
}

// flagUsages are the usage lines of fs's flags that are the driver's, or
// that are warden's.
func flagUsages(fs *pflag.FlagSet, driverOnes bool) string {
	sub := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && (f.Annotations[driverAnnotation] != nil) == driverOnes {
			sub.AddFlag(f)
		}
	})
	return strings.TrimRight(sub.FlagUsages(), " \n")
}

// driverFlagsHelp is help's section of the selected driver's flags on c, or
// the line that says why they are not shown.
func driverFlagsHelp(c *cobra.Command) string {
	all := pflag.NewFlagSet("", pflag.ContinueOnError)
	all.AddFlagSet(c.LocalFlags())
	all.AddFlagSet(c.InheritedFlags())
	if u := flagUsages(all, true); u != "" {
		return fmt.Sprintf("Flags of driver %q:\n%s", driverFlags.sel.name(), u)
	}
	if driverFlags.err != nil && contractCommand(strings.Fields(
		strings.TrimPrefix(c.CommandPath(), "warden "))) != "" {
		return "Flags of the driver: not shown: " +
			driver.EscapeOutside(driverFlags.err.Error())
	}
	return ""
}
