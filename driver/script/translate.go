package script

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Result holds the translated script and base image from a Containerfile.
type Result struct {
	Image  string
	Script string
}

type builder struct {
	image   string
	sb      strings.Builder
	env     map[string]string
	workdir string
}

// Translate converts a Dockerfile/Containerfile into a shell script.
// COPY directives emit `warden-io fetch` commands to pull files through
// the relay. The resulting script is portable across drivers and platforms.
func Translate(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b := &builder{env: make(map[string]string)}
	b.sb.WriteString("#!/bin/sh\nset -e\n")

	scanner := bufio.NewScanner(f)
	var continuation string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasSuffix(line, "\\") {
			continuation += strings.TrimSuffix(line, "\\") + "\\\n"
			continue
		}
		if continuation != "" {
			line = continuation + line
			continuation = ""
		}

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		directive, rest := splitDirective(line)
		if err := b.handleDirective(directive, rest); err != nil {
			return nil, err
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading containerfile: %w", err)
	}
	if b.image == "" {
		return nil, fmt.Errorf("no FROM directive found")
	}

	return &Result{
		Image:  b.image,
		Script: b.sb.String(),
	}, nil
}

func (b *builder) handleDirective(directive, rest string) error {
	switch directive {
	case "FROM":
		if b.image != "" {
			return fmt.Errorf(
				"multi-stage builds not yet supported (second FROM)")
		}
		b.image = parseFrom(rest)
	case "RUN":
		b.sb.WriteString(rest + "\n")
	case "ENV":
		for k, v := range parseEnv(rest) {
			b.env[k] = v
			fmt.Fprintf(&b.sb, "export %s=%s\n", k, shellQuote(v))
		}
	case "WORKDIR":
		dir := expandEnv(strings.TrimSpace(rest), b.env)
		b.workdir = dir
		fmt.Fprintf(&b.sb, "mkdir -p %s && cd %s\n",
			shellQuote(dir), shellQuote(dir))
	case "COPY":
		return b.handleCopy(rest)
	case "ARG":
		k, v := parseArg(rest)
		if v != "" {
			b.env[k] = v
			fmt.Fprintf(&b.sb,
				": ${%s:=%s}\nexport %s\n", k, shellQuote(v), k)
		}
	default:
		return rejectOrIgnore(directive)
	}
	return nil
}

func (b *builder) handleCopy(rest string) error {
	if strings.Contains(rest, "--from=") {
		return fmt.Errorf("COPY --from= (multi-stage) not supported")
	}
	if strings.HasPrefix(rest, ".warden ") ||
		strings.HasPrefix(rest, ".warden\t") {
		return nil
	}

	chown, chmod, positional := parseCopyFlags(rest)
	if len(positional) < 2 {
		return fmt.Errorf("COPY requires source and destination")
	}

	dest := positional[len(positional)-1]
	sources := positional[:len(positional)-1]

	// Single source to a non-directory destination
	if len(sources) == 1 && !strings.HasSuffix(dest, "/") &&
		!isGlobOrDir(sources[0]) {
		dir := ""
		if idx := strings.LastIndex(dest, "/"); idx > 0 {
			dir = dest[:idx]
		}
		if dir != "" {
			fmt.Fprintf(&b.sb, "mkdir -p %s\n", shellQuote(dir))
		}
		fmt.Fprintf(&b.sb, "warden-io fetch %s -o %s\n",
			shellQuote(sources[0]), shellQuote(dest))
	} else {
		// Multiple sources or directory/glob → fetch to directory
		if !strings.HasSuffix(dest, "/") {
			dest += "/"
		}
		fmt.Fprintf(&b.sb, "mkdir -p %s\n", shellQuote(dest))
		for _, src := range sources {
			if isGlobOrDir(src) {
				// Directory or glob: use warden-io directory fetch
				fmt.Fprintf(&b.sb,
					"warden-io fetch %s -o %s\n",
					shellQuote(ensureTrailingSlash(src)),
					shellQuote(dest))
			} else {
				fmt.Fprintf(&b.sb,
					"warden-io fetch %s -o %s\n",
					shellQuote(src),
					shellQuote(dest+src))
			}
		}
	}

	if chown != "" {
		fmt.Fprintf(&b.sb, "chown -R %s %s\n", chown, shellQuote(dest))
	}
	if chmod != "" {
		fmt.Fprintf(&b.sb, "chmod -R %s %s\n", chmod, shellQuote(dest))
	}
	return nil
}

func isGlobOrDir(s string) bool {
	return strings.ContainsAny(s, "*?[") || strings.HasSuffix(s, "/")
}

func ensureTrailingSlash(s string) string {
	if strings.HasSuffix(s, "/") {
		return s
	}
	return s + "/"
}

func rejectOrIgnore(directive string) error {
	switch directive {
	case "USER":
		return fmt.Errorf(
			"USER directive not supported in script mode")
	case "SHELL":
		return fmt.Errorf(
			"SHELL directive not supported in script mode")
	case "ADD":
		return fmt.Errorf(
			"ADD not supported (use COPY for local files, RUN for remote)")
	case "EXPOSE", "VOLUME", "LABEL", "STOPSIGNAL",
		"HEALTHCHECK", "ONBUILD", "ENTRYPOINT", "CMD":
		return nil
	default:
		return fmt.Errorf("unsupported directive: %s", directive)
	}
}

func splitDirective(line string) (string, string) {
	idx := strings.IndexAny(line, " \t")
	if idx < 0 {
		return strings.ToUpper(line), ""
	}
	return strings.ToUpper(line[:idx]), strings.TrimSpace(line[idx+1:])
}

func parseFrom(rest string) string {
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return rest
	}
	return fields[0]
}

func parseCopyFlags(args string) (chown, chmod string, positional []string) {
	for _, p := range strings.Fields(args) {
		switch {
		case strings.HasPrefix(p, "--chown="):
			chown = strings.TrimPrefix(p, "--chown=")
		case strings.HasPrefix(p, "--chmod="):
			chmod = strings.TrimPrefix(p, "--chmod=")
		default:
			positional = append(positional, p)
		}
	}
	return
}

var envPairRe = regexp.MustCompile(`^(\w+)=(.*)$`)

func parseEnv(rest string) map[string]string {
	result := make(map[string]string)
	fields := splitEnvFields(rest)
	for _, field := range fields {
		if m := envPairRe.FindStringSubmatch(field); m != nil {
			result[m[1]] = m[2]
		}
	}
	if len(result) == 0 {
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] =
				strings.TrimSpace(parts[1])
		}
	}
	return result
}

func splitEnvFields(s string) []string {
	var fields []string
	var current strings.Builder
	inQuote := byte(0)

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case inQuote != 0:
			current.WriteByte(ch)
			if ch == inQuote {
				inQuote = 0
			}
		case ch == '"' || ch == '\'':
			current.WriteByte(ch)
			inQuote = ch
		case ch == ' ' || ch == '\t':
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
		case ch == '\\' && i+1 < len(s) && s[i+1] == '\n':
			i++
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields
}

func parseArg(rest string) (string, string) {
	rest = strings.TrimSpace(rest)
	if idx := strings.Index(rest, "="); idx >= 0 {
		return rest[:idx], rest[idx+1:]
	}
	return rest, ""
}

func expandEnv(s string, env map[string]string) string {
	return os.Expand(s, func(key string) string {
		if v, ok := env[key]; ok {
			return v
		}
		return ""
	})
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`!#&|;(){}[]<>?*~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
