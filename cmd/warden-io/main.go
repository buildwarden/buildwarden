package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "fetch":
		if len(os.Args) < 3 {
			fatal("fetch requires at least one argument")
		}
		os.Exit(runFetch(os.Args[2:]))
	case "post":
		if len(os.Args) < 3 {
			fatal("post requires a file argument")
		}
		os.Exit(runPost(os.Args[2:]))
	case "trust":
		os.Exit(runTrust())
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: warden-io <fetch|post|trust> [args...]\n")
	fmt.Fprintf(os.Stderr, "  fetch <file> [-o dest]  "+
		"Fetch a context file from relay\n")
	fmt.Fprintf(os.Stderr, "  post <file> [name]      "+
		"Post a build artifact to relay\n")
	fmt.Fprintf(os.Stderr, "  trust                   "+
		"Install relay CA into system trust store\n")
	os.Exit(1)
}

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "warden-io: %s\n", msg)
	os.Exit(1)
}

func runFetch(args []string) int {
	var dest string
	var files []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			i++
			if i >= len(args) {
				fatal("-o requires a destination path")
			}
			dest = args[i]
		default:
			files = append(files, args[i])
		}
	}

	if len(files) == 0 {
		fatal("no files specified")
	}

	exitCode := 0
	for _, f := range files {
		// Directory fetch: list + download all
		if strings.HasSuffix(f, "/") || f == "." {
			if err := fetchDir(f, dest); err != nil {
				fmt.Fprintf(os.Stderr, "warden-io: fetch %s: %s\n", f, err)
				exitCode = 1
			}
			continue
		}

		out := dest
		if out == "" {
			out = f
		} else if len(files) > 1 || strings.HasSuffix(out, "/") {
			out = filepath.Join(out, filepath.Base(f))
		}

		if err := fetchFile(f, out); err != nil {
			fmt.Fprintf(os.Stderr, "warden-io: fetch %s: %s\n", f, err)
			exitCode = 1
		}
	}
	return exitCode
}

func fetchDir(prefix, destDir string) error {
	if prefix == "." {
		prefix = ""
	}
	listing, err := fetchListing(prefix)
	if err != nil {
		return err
	}

	for _, f := range listing {
		out := f
		if destDir != "" {
			// Strip the prefix from the source to get relative path
			rel := f
			if prefix != "" {
				rel = strings.TrimPrefix(f, prefix)
			}
			out = filepath.Join(destDir, rel)
		}
		if err := fetchFile(f, out); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
	}
	return nil
}

func fetchListing(prefix string) ([]string, error) {
	url := "http://cwd/" + prefix
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func fetchFile(src, dest string) error {
	if dir := filepath.Dir(dest); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	resp, err := http.Get("http://cwd/" + src)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func runTrust() int {
	resp, err := http.Get("http://artifacts/ca.pem")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: trust: %s\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "warden-io: trust: HTTP %d\n", resp.StatusCode)
		return 1
	}

	pem, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: trust: reading CA: %s\n", err)
		return 1
	}

	installed := false

	// Alpine/Debian/Ubuntu: /usr/local/share/ca-certificates/ + update-ca-certificates
	dir := "/usr/local/share/ca-certificates"
	if err := os.MkdirAll(dir, 0755); err == nil {
		certPath := filepath.Join(dir, "warden-ca.crt")
		if err := os.WriteFile(certPath, pem, 0644); err == nil {
			installed = true
		}
	}

	// Append to bundle (works without update-ca-certificates)
	for _, bundle := range []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
		"/etc/ssl/cert.pem",
	} {
		if f, err := os.OpenFile(bundle, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			_, _ = f.Write([]byte("\n"))
			_, _ = f.Write(pem)
			f.Close()
			installed = true
			break
		}
	}

	if !installed {
		fmt.Fprintf(os.Stderr, "warden-io: trust: could not install CA\n")
		return 1
	}

	return 0
}

func runPost(args []string) int {
	filePath := args[0]
	name := filepath.Base(filePath)
	if len(args) > 1 {
		name = args[1]
	}

	f, err := os.Open(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: %s\n", err)
		return 1
	}
	defer f.Close()

	resp, err := http.Post("http://artifacts/"+name, "application/octet-stream", f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: post %s: %s\n", name, err)
		return 1
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "warden-io: post %s: HTTP %d: %s\n",
			name, resp.StatusCode, strings.TrimSpace(string(body)))
		return 1
	}

	fmt.Print(string(body))
	return 0
}
