package firecracker

import (
	"bytes"
	"io"
	"os"
	"strings"
)

// prefixWriter writes each line to w with a prefix, so the two VMs' consoles
// stay apart on stderr under --verbose.
type prefixWriter struct {
	prefix  string
	w       io.Writer
	partial bool
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	var out bytes.Buffer
	for _, line := range strings.SplitAfter(string(b), "\n") {
		if line == "" {
			continue
		}
		if !p.partial {
			out.WriteString(p.prefix)
		}
		out.WriteString(line)
		p.partial = !strings.HasSuffix(line, "\n")
	}
	if _, err := p.w.Write(out.Bytes()); err != nil {
		return 0, err
	}
	return len(b), nil
}

// consoleTail returns the last n lines of a console file, for a failed
// build's error ("" when there is none).
func consoleTail(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
