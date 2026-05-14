package container

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"warden/driver"
)

// editContainerfile creates a rewritten copy of the Containerfile
// in the .warden directory. The original is never modified.
func editContainerfile(
	containerfile string,
	contextDir string,
	extensions []driver.Extension,
) error {
	origfile, err := os.Open(containerfile)
	if err != nil {
		return fmt.Errorf(
			"error opening containerfile: %w", err)
	}
	defer origfile.Close()

	ctrfilePath := filepath.Join(
		contextDir, wardenDir, "Containerfile")
	ctrfile, err := os.Create(ctrfilePath)
	if err != nil {
		return fmt.Errorf(
			"error creating edited containerfile: %w", err)
	}
	defer ctrfile.Close()

	env := make(map[string]string)
	for _, ext := range extensions {
		extenv := ext.Env()
		if extenv == nil {
			continue
		}
		for k, v := range extenv {
			if _, ok := env[k]; ok {
				return fmt.Errorf("env collision: %s", k)
			}
			env[k] = v
		}
	}
	var enventries []string
	for k, v := range env {
		enventries = append(enventries,
			fmt.Sprintf("%s=%s", k, v))
	}

	scanner := bufio.NewScanner(origfile)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "FROM ") {
			_, _ = ctrfile.WriteString(line + "\n")
			if len(enventries) > 0 {
				_, _ = ctrfile.WriteString("ENV " +
					strings.Join(enventries,
						" \\\n") + "\n")
			}
			_, _ = ctrfile.WriteString(
				"COPY .warden /.warden\n")
			_, _ = ctrfile.WriteString(
				"RUN ln -sf /.warden/warden-io" +
					" /usr/local/bin/warden-io" +
					" && find /.warden/ext.d/" +
					" -exec sh {} \\;\n")
			continue
		}

		if isCopyDirective(line) {
			rewritten, err := rewriteCopy(
				line, contextDir)
			if err != nil {
				return fmt.Errorf(
					"error rewriting COPY: %w", err)
			}
			_, _ = ctrfile.WriteString(rewritten + "\n")
			continue
		}

		_, _ = ctrfile.WriteString(line + "\n")
	}

	return nil
}

func isCopyDirective(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "COPY ") ||
		strings.HasPrefix(trimmed, "COPY\t")
}

func rewriteCopy(
	line string, contextDir string,
) (string, error) {
	trimmed := strings.TrimSpace(line)
	args := trimmed[5:] // strip "COPY "

	if strings.Contains(args, "--from=") {
		return line, nil
	}

	chown, chmod, positional := parseCopyFlags(args)
	if len(positional) < 2 {
		return line, nil
	}

	dest := positional[len(positional)-1]
	sources := positional[:len(positional)-1]

	var files []string
	for _, src := range sources {
		expanded, err := expandContextPath(
			src, contextDir)
		if err != nil {
			return "", err
		}
		files = append(files, expanded...)
	}

	if len(files) == 0 {
		return "# (empty COPY: no matching files)", nil
	}

	run := buildFetchRun(files, dest)
	if chown != "" {
		run += fmt.Sprintf(
			" && \\\n    chown -R %s %s", chown, dest)
	}
	if chmod != "" {
		run += fmt.Sprintf(
			" && \\\n    chmod -R %s %s", chmod, dest)
	}
	return run, nil
}

func parseCopyFlags(
	args string,
) (chown, chmod string, positional []string) {
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

func buildFetchRun(files []string, dest string) string {
	if len(files) == 1 && !strings.HasSuffix(dest, "/") {
		dir := dest[:strings.LastIndex(dest, "/")+1]
		if dir != "" {
			return fmt.Sprintf(
				"RUN mkdir -p %s && "+
					"warden-io fetch %s -o %s",
				dir, files[0], dest)
		}
		return fmt.Sprintf(
			"RUN warden-io fetch %s -o %s",
			files[0], dest)
	}

	d := dest
	if !strings.HasSuffix(d, "/") {
		d += "/"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb,
		"RUN mkdir -p %s && \\\n    printf '%%s\\n'",
		dest)
	for _, f := range files {
		fmt.Fprintf(&sb, " '%s'", f)
	}
	fmt.Fprintf(&sb,
		" | \\\n    xargs -P8 -I{} sh -c "+
			"'mkdir -p \"%s$(dirname \"{}\")\" && "+
			"warden-io fetch \"{}\" -o \"%s{}\"'",
		d, d)
	return sb.String()
}

// expandContextPath resolves a source path or glob against the
// build context directory.
func expandContextPath(
	src string, contextDir string,
) ([]string, error) {
	fullPattern := filepath.Join(contextDir, src)

	info, err := os.Stat(fullPattern)
	if err == nil && info.IsDir() {
		var files []string
		err = filepath.Walk(fullPattern, func(
			path string, fi os.FileInfo, walkErr error,
		) error {
			if walkErr != nil {
				return walkErr
			}
			if fi.IsDir() {
				switch fi.Name() {
				case wardenDir, ".git":
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(contextDir, path)
			files = append(files, rel)
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
		if strings.HasPrefix(rel, wardenDir+"/") ||
			strings.HasPrefix(rel, ".git/") {
			continue
		}
		files = append(files, rel)
	}
	return files, nil
}
