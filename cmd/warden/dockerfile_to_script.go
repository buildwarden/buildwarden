package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type ScriptResult struct {
	Image  string
	Script string
}

type scriptBuilder struct {
	image   string
	sb      strings.Builder
	env     map[string]string
	workdir string
}

func dockerfileToScript(path string) (*ScriptResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b := &scriptBuilder{env: make(map[string]string)}
	b.sb.WriteString("#!/bin/sh\nset -e\n")

	scanner := bufio.NewScanner(f)
	var continuation string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasSuffix(line, "\\") {
			continuation += strings.TrimSuffix(line, "\\") + "\n"
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
		return nil, fmt.Errorf("reading dockerfile: %w", err)
	}
	if b.image == "" {
		return nil, fmt.Errorf("no FROM directive found")
	}

	return &ScriptResult{Image: b.image, Script: b.sb.String()}, nil
}

func (b *scriptBuilder) handleDirective(
	directive, rest string,
) error {
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
			fmt.Fprintf(&b.sb, ": ${%s:=%s}\nexport %s\n",
				k, shellQuote(v), k)
		}
	default:
		return rejectOrIgnore(directive, rest)
	}
	return nil
}

func (b *scriptBuilder) handleCopy(rest string) error {
	if strings.Contains(rest, "--from=") {
		return fmt.Errorf("COPY --from= (multi-stage) not supported")
	}
	if strings.HasPrefix(rest, ".warden ") ||
		strings.HasPrefix(rest, ".warden\t") {
		return nil
	}
	return fmt.Errorf("unexpected COPY directive " +
		"(should be rewritten before script generation)")
}

func rejectOrIgnore(directive, rest string) error {
	switch directive {
	case "USER":
		return fmt.Errorf(
			"USER directive not supported in script mode "+
				"(build runs as container default user): %s", rest)
	case "SHELL":
		return fmt.Errorf(
			"SHELL directive not supported in script mode")
	case "ADD":
		return fmt.Errorf(
			"ADD not supported (use COPY for local files, RUN for remote)")
	case "EXPOSE", "VOLUME", "LABEL", "STOPSIGNAL", "HEALTHCHECK",
		"ONBUILD", "ENTRYPOINT", "CMD":
		return nil
	default:
		return fmt.Errorf("unsupported directive: %s", directive)
	}
}

func splitDirective(line string) (string, string) {
	// Handle cases like "RUN command" or "ENV KEY=value"
	idx := strings.IndexAny(line, " \t")
	if idx < 0 {
		return strings.ToUpper(line), ""
	}
	return strings.ToUpper(line[:idx]), strings.TrimSpace(line[idx+1:])
}

// extractFromImage reads a Dockerfile and returns just the FROM image,
// ignoring all other directives. Used when we need the image name before
// the full Dockerfile-to-script translation (which requires COPY rewriting).
func extractFromImage(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		directive, rest := splitDirective(line)
		if directive == "FROM" {
			return parseFrom(rest), nil
		}
	}
	return "", fmt.Errorf("no FROM directive found in %s", path)
}

func parseFrom(rest string) string {
	// FROM image:tag AS name
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return rest
	}
	return fields[0]
}

var envPairRe = regexp.MustCompile(`^(\w+)=(.*)$`)

func parseEnv(rest string) map[string]string {
	result := make(map[string]string)
	// Two forms: ENV KEY=VALUE ... or ENV KEY VALUE (legacy, single pair)
	fields := splitEnvFields(rest)
	for _, field := range fields {
		if m := envPairRe.FindStringSubmatch(field); m != nil {
			result[m[1]] = m[2]
		}
	}
	if len(result) == 0 {
		// Legacy form: ENV KEY VALUE
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
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
			i++ // skip continuation
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
	// If no special chars, return as-is.
	if !strings.ContainsAny(s, " \t\n'\"\\$`!#&|;(){}[]<>?*~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

