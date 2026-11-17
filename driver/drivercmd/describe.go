package drivercmd

import (
	"fmt"
	"slices"
)

// Description is what describe prints (v1.md, describe).
type Description struct {
	Contract     []int             `json:"contract"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Vendor       string            `json:"vendor,omitempty"`
	BugURL       string            `json:"bug_url,omitempty"`
	Commands     []string          `json:"commands"`
	Capabilities Capabilities      `json:"capabilities"`
	Flags        []DescribeFlag    `json:"flags"`
	Keys         []DescribeKey     `json:"keys"`
	Ext          []ExtCommand      `json:"ext"`
	Help         map[string]string `json:"help,omitempty"`
}

// DescribeFlag is one entry of describe's flags.
type DescribeFlag struct {
	Name     string   `json:"name"`
	Value    bool     `json:"value"`
	Commands []string `json:"commands"`
	Usage    string   `json:"usage"`
}

// DescribeKey is one entry of describe's keys.
type DescribeKey struct {
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Aliases  []string `json:"aliases,omitempty"`
	Commands []string `json:"commands"`
	Usage    string   `json:"usage"`
}

// ReservedFlags are the orchestrator's flag names by command, which no
// driver may declare for that command (v1.md, Reserved flag names).
// The entry under "*" holds the names reserved on every command.
var ReservedFlags = map[string][]string{
	everyCommand: {"driver", "config", "verbose", "color", "help", "version"},
	CmdRun: {"capture", "output", "no-compress", "timeout", "tmpdir", "quiet", "guest-os",
		"script", "image", "cpus", "memory", "keep-vms", "build-link"},
	CmdDoctor:       {"build-link"},
	CmdImagePrepare: {"os", "source"},
	CmdClean:        {"cache"},
}

// Description builds d's describe from its Go value: the commands whose
// function is set (or Describe.Commands), and the flags and keys of its
// settings. It refuses a value describe could not report truthfully: a
// setting that breaks the contract's rules, a flag of an orchestrator's name
// or of a command the driver does not list (DRV-C1, DRV-C4), and
// capabilities that do not match the commands (DRV-37).
func (d *Driver) Description() (*Description, error) {
	ds := &d.Describe
	if err := checkSettings(ds.Name, ds.Settings); err != nil {
		return nil, fmt.Errorf("driver %q declares its settings wrongly: %w", ds.Name, err)
	}
	listed := d.commands()
	has := func(c string) bool { return slices.Contains(listed, c) }
	switch {
	case !has(CmdRun):
		return nil, fmt.Errorf("driver %q has no run", ds.Name)
	case ds.Capabilities.Shell != has(CmdAttach):
		return nil, fmt.Errorf("driver %q: capabilities.shell is %v but attach is "+
			"listed: %v", ds.Name, ds.Capabilities.Shell, has(CmdAttach))
	case (len(ds.Ext) > 0) != has(CmdExt):
		return nil, fmt.Errorf("driver %q: %d ext commands but ext is listed: %v",
			ds.Name, len(ds.Ext), has(CmdExt))
	case (len(ds.Capabilities.ImageOS) > 0) != has(CmdImagePrepare):
		return nil, fmt.Errorf("driver %q: capabilities.image_os is %v but "+
			"image-prepare is listed: %v", ds.Name, ds.Capabilities.ImageOS,
			has(CmdImagePrepare))
	}
	out := &Description{Contract: ContractVersions, Name: ds.Name, Version: ds.Version,
		Vendor: ds.Vendor, BugURL: ds.BugURL, Commands: listed,
		Capabilities: ds.Capabilities, Flags: []DescribeFlag{}, Keys: []DescribeKey{},
		Ext: ds.Ext, Help: ds.Help}
	if out.Ext == nil {
		out.Ext = []ExtCommand{}
	}
	for _, s := range ds.Settings {
		if err := checkListed(&s, listed); err != nil {
			return nil, fmt.Errorf("driver %q, setting %q: %w", ds.Name, s.Name, err)
		}
		if s.Flag != "" {
			f, err := describeFlag(&s, listed)
			if err != nil {
				return nil, fmt.Errorf("driver %q: %w", ds.Name, err)
			}
			out.Flags = append(out.Flags, f)
		}
		if k := s.key(); k != "" {
			all := s.expand(listed, func(string) bool { return true })
			out.Keys = append(out.Keys, DescribeKey{Key: k, Type: s.Type.tomlType(),
				Aliases: s.Aliases, Commands: all, Usage: s.Usage})
		}
	}
	return out, nil
}

// describeFlag is s's entry in describe's flags, for a driver that lists
// listed. It refuses a flag of no command that takes flags, and one of a name
// the orchestrator uses on one of its commands (DRV-C1).
func describeFlag(s *Setting, listed []string) (DescribeFlag, error) {
	cmds := s.expand(listed, func(c string) bool { return slices.Contains(flagCommands, c) })
	if len(cmds) == 0 {
		return DescribeFlag{}, fmt.Errorf("flag --%s names no command that takes flags",
			s.Flag)
	}
	for _, c := range append([]string{everyCommand}, cmds...) {
		if slices.Contains(ReservedFlags[c], s.Flag) {
			return DescribeFlag{}, fmt.Errorf("flag --%s is the orchestrator's on %s",
				s.Flag, c)
		}
	}
	return DescribeFlag{Name: s.Flag, Value: s.Type != Bool, Commands: cmds,
		Usage: s.Usage}, nil
}

// checkListed refuses a command of s that the driver does not list.
func checkListed(s *Setting, listed []string) error {
	for _, c := range s.Commands {
		if c != everyCommand && !slices.Contains(listed, c) {
			return fmt.Errorf("names %s, which the driver does not list", c)
		}
		if c != everyCommand && s.Flag != "" && s.key() == "" &&
			!slices.Contains(flagCommands, c) {
			return fmt.Errorf("flag --%s names %s, which takes no flags", s.Flag, c)
		}
	}
	return nil
}
