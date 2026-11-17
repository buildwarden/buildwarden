package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// config is the fake's settings for one command, each with the place it
// came from.
type config struct {
	greeting, greetingFrom string
	shout                  bool
	refresh, refreshFrom   string
}

// readConfig reads the fake's settings for command, by the contract's
// rules (DRV-C2 to DRV-C8): its flags from args, then its keys from the one
// file the request names, its own table before [driver].
func readConfig(command, file string, args []string, probe bool) (*config, error) {
	cfg := &config{greeting: "hello", greetingFrom: "the default"}
	if err := cfg.fromArgs(command, args); err != nil {
		return nil, err
	}
	switch {
	case file != "":
	case fault == faultCwdConfig:
		if _, err := os.Stat("warden.toml"); err == nil {
			file = "warden.toml"
		}
	case fault == faultWardenConfig:
		file = os.Getenv("WARDEN_CONFIG")
	}
	if command == "reap" && fault == faultReapConfigGone && file != "" {
		if _, err := os.Stat(file); err != nil {
			return nil, &fixable{msg: fmt.Sprintf("config file %s: %v", file, err)}
		}
	}
	if file == "" || !slices.Contains(greetingCommands, command) {
		return cfg, nil
	}
	tree, err := load(file)
	if err != nil {
		return nil, err
	}
	if err := cfg.fromFile(command, file, tree); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *config) fromArgs(command string, args []string) error {
	for _, a := range args {
		v, isGreeting := strings.CutPrefix(a, "--greeting=")
		switch {
		case isGreeting && slices.Contains(greetingCommands, command):
			if v != "" {
				c.greeting, c.greetingFrom = v, "--greeting"
			}
		case a == "--shout" && command == "run":
			c.shout = true
		case fault == faultAcceptArg:
		default:
			return &fixable{msg: fmt.Sprintf("unknown flag %s for driver %q on %s", a,
				name,
				command),
				fix: "remove it; the fake's flags are --greeting and --shout"}
		}
	}
	return nil
}

func load(file string) (map[string]any, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &fixable{msg: fmt.Sprintf("config file %s does not exist", file),
			fix: "create it, or name a config file that exists"}
	}
	if err != nil {
		return nil, &fixable{msg: fmt.Sprintf("reading config file %s: %v", file, err)}
	}
	var tree map[string]any
	if _, err := toml.Decode(string(data), &tree); err != nil {
		return nil, &fixable{msg: fmt.Sprintf("parsing %s: %v", file, err),
			fix: "fix the TOML syntax in " + file}
	}
	return tree, nil
}

func table(tree map[string]any, path ...string) map[string]any {
	cur := tree
	for _, p := range path {
		next, ok := cur[p].(map[string]any)
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func (c *config) fromFile(command, file string, tree map[string]any) error {
	if fault == faultRefuseUnowned {
		for k := range tree {
			if k != "driver" {
				return &fixable{msg: file + ": unknown table [" + k + "]"}
			}
		}
	}
	own, common := table(tree, "driver", name), table(tree, "driver")
	if err := c.greetingFromFile(file, own); err != nil {
		return err
	}
	if fault == faultUnlistedCommon {
		if v, ok := common["image_min_grace"]; ok {
			if _, err := age(v); err != nil {
				msg := fmt.Sprintf("%s [driver] image_min_grace: %v", file, err)
				return &fixable{msg: msg}
			}
		}
	}
	if command != "run" {
		return nil
	}
	places := []struct {
		t     map[string]any
		label string
	}{{own, "[driver.fake]"}, {common, "[driver]"}}
	if fault == faultDriverFirst {
		slices.Reverse(places)
	}
	for _, p := range places {
		v, ok := p.t["image_refresh_after"]
		if !ok {
			continue
		}
		s, err := age(v)
		if err != nil && fault != faultWrongType {
			return &fixable{msg: fmt.Sprintf("%s %s image_refresh_after: %v", file,
				p.label, err), fix: `set it to an age such as "7d" or "36h"`}
		}
		c.refresh, c.refreshFrom = s, file+" "+p.label+" image_refresh_after"
		break
	}
	return nil
}

// greetingFromFile reads [driver.fake] greeting when no flag set it.
func (c *config) greetingFromFile(file string, own map[string]any) error {
	v, ok := own["greeting"]
	if !ok || c.greetingFrom == "--greeting" {
		return nil
	}
	s, isString := v.(string)
	if !isString && fault != faultWrongType {
		return &fixable{msg: fmt.Sprintf("%s [driver.fake] greeting: want a string, "+
			"not %T", file, v), fix: "set it to a string"}
	}
	if !isString {
		s = fmt.Sprint(v)
	}
	c.greeting, c.greetingFrom = s, file+" [driver.fake] greeting"
	return nil
}

// age checks that v is a string holding an age: a Go duration, or a number
// of days followed by d. An empty string is the driver's default.
func age(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("want a string holding an age, not %T", v)
	}
	if s == "" {
		return s, nil
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		if n, err := strconv.ParseFloat(days, 64); err == nil && n >= 0 {
			return s, nil
		}
	} else if d, err := time.ParseDuration(s); err == nil && d >= 0 {
		return s, nil
	}
	return "", fmt.Errorf("invalid age %q", s)
}

// echo prints each setting and its place on stderr, which is how the
// suite's self-test sees which place won (DRV-C8).
func (c *config) echo() {
	fmt.Fprintf(os.Stderr, "warden-driver-fake: greeting=%q from %s\n", c.greeting,
		c.greetingFrom)
	from := c.refreshFrom
	if from == "" {
		from = "the default"
	}
	fmt.Fprintf(os.Stderr, "warden-driver-fake: image_refresh_after=%q from %s\n",
		c.refresh, from)
}
