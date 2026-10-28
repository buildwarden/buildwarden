// Package script translates a Containerfile into the POSIX shell build script
// every driver's Linux guest runs. Translation happens on the host because it
// needs the build context: COPY sources (files, directories, globs) are
// expanded against it here and become explicit `warden-io fetch` commands, so
// the guest fetches exactly the files the host resolved, each through the
// relay (BOOT-04).
package script

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Result holds the translated script and base image from a Containerfile.
type Result struct {
	Image  string
	Script string
}

type builder struct {
	contextDir string
	image      string
	sb         strings.Builder
	env        map[string]string
}

// Translate converts the Containerfile at path into a shell script, resolving
// COPY sources against contextDir (the current directory when empty).
func Translate(path, contextDir string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if contextDir == "" {
		contextDir = "."
	}
	b := &builder{contextDir: contextDir, env: make(map[string]string)}
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
			fmt.Fprintf(&b.sb, "export %s=%s\n", k, shellQuoteEnv(v))
		}
	case "WORKDIR":
		dir := expandEnv(strings.TrimSpace(rest), b.env)
		fmt.Fprintf(&b.sb, "mkdir -p %s\ncd %s\n",
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
		return rejectOrIgnore(directive, rest)
	}
	return nil
}

// fetchFile is one file a COPY fetches: its context path and its path under
// the destination directory.
type fetchFile struct {
	contextPath string
	destPath    string
}

// handleCopy expands the COPY sources against the build context and writes one
// fetch per file. A file (or glob match) lands in a directory dest by its base
// name, as docker places it; a directory's files keep their path under it.
func (b *builder) handleCopy(rest string) error {
	if strings.Contains(rest, "--from=") {
		return fmt.Errorf("COPY --from= (multi-stage) not supported")
	}
	chown, chmod, positional := parseCopyFlags(rest)
	if len(positional) < 2 {
		return fmt.Errorf("COPY requires source and destination")
	}
	dest := positional[len(positional)-1]
	files, anyDir, err := b.resolveCopySources(positional[:len(positional)-1])
	if err != nil {
		return err
	}
	if len(files) == 0 {
		b.sb.WriteString("# (empty COPY: no matching files)\n")
		return nil
	}
	// Several files, or a directory source (even one holding a single file),
	// copy into dest as a directory.
	if anyDir || len(files) > 1 {
		dest = strings.TrimSuffix(dest, "/") + "/"
	}
	b.writeFetches(files, dest)

	if chown != "" {
		fmt.Fprintf(&b.sb, "chown -R %s %s\n", chown, shellQuote(dest))
	}
	if chmod != "" {
		fmt.Fprintf(&b.sb, "chmod -R %s %s\n", chmod, shellQuote(dest))
	}
	return nil
}

// resolveCopySources expands COPY sources against the build context. anyDir
// reports whether any source is a directory.
func (b *builder) resolveCopySources(sources []string) (files []fetchFile, anyDir bool,
	err error) {
	for _, src := range sources {
		expanded, err := expandContextPath(src, b.contextDir)
		if err != nil {
			return nil, false, err
		}
		// Context paths are slash-separated (they are fetch paths and paths
		// in the Linux guest), whatever the host's separator.
		srcClean := path.Clean(filepath.ToSlash(src))
		srcDir := isDir(filepath.Join(b.contextDir, src))
		anyDir = anyDir || srcDir
		for _, f := range expanded {
			rel := path.Base(f)
			if srcDir {
				rel = strings.TrimPrefix(f, srcClean+"/")
			}
			files = append(files, fetchFile{contextPath: f, destPath: rel})
		}
	}
	return files, anyDir, nil
}

// writeFetches writes the fetch commands: one file to a file dest, or each
// file under a directory dest (ending in "/").
func (b *builder) writeFetches(files []fetchFile, dest string) {
	if !strings.HasSuffix(dest, "/") {
		if dir := path.Dir(dest); dir != "." && dir != "/" {
			fmt.Fprintf(&b.sb, "mkdir -p %s\n", shellQuote(dir))
		}
		fmt.Fprintf(&b.sb, "warden-io fetch %s -o %s\n",
			shellQuote(files[0].contextPath), shellQuote(dest))
	} else {
		fmt.Fprintf(&b.sb, "mkdir -p %s\n", shellQuote(dest))
		for _, f := range files {
			out := dest + f.destPath
			if dir := path.Dir(out); dir+"/" != dest {
				fmt.Fprintf(&b.sb, "mkdir -p %s\n", shellQuote(dir))
			}
			fmt.Fprintf(&b.sb, "warden-io fetch %s -o %s\n",
				shellQuote(f.contextPath), shellQuote(out))
		}
	}
}

// expandContextPath resolves a source path or glob against the build context
// directory, returning slash-separated context paths of the files it names.
// .git is never copied.
func expandContextPath(src, contextDir string) ([]string, error) {
	fullPattern := filepath.Join(contextDir, src)

	info, err := os.Stat(fullPattern)
	if err == nil && info.IsDir() {
		var files []string
		err = filepath.Walk(fullPattern, func(
			p string, fi os.FileInfo, walkErr error,
		) error {
			if walkErr != nil {
				return walkErr
			}
			if fi.IsDir() {
				if fi.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(contextDir, p)
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		return files, err
	}

	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || info.IsDir() {
			continue
		}
		rel, _ := filepath.Rel(contextDir, m)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, ".git/") {
			continue
		}
		files = append(files, rel)
	}
	return files, nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func rejectOrIgnore(directive, rest string) error {
	switch directive {
	case "USER":
		return fmt.Errorf(
			"USER directive not supported in script mode "+
				"(build runs as root): %s", rest)
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
			result[m[1]] = unquote(m[2])
		}
	}
	if len(result) == 0 {
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] =
				unquote(strings.TrimSpace(parts[1]))
		}
	}
	return result
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
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
	if !strings.ContainsAny(s, " \t\n'\"\\$`!#&|;(){}[]<>?*~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// shellQuoteEnv quotes for ENV values, using double quotes to preserve $VAR
// expansion (matching Dockerfile ENV semantics).
func shellQuoteEnv(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\`!#&|;(){}[]<>?*~$") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch c {
		case '"', '\\', '`':
			b.WriteByte('\\')
			b.WriteRune(c)
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
