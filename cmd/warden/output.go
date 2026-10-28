package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime"

	"github.com/fatih/color"
)

type Logger struct {
	out io.Writer
}

var log = &Logger{out: os.Stderr}

var prefixWarden = color.New(color.FgCyan).Sprint("[warden]")

func (l *Logger) Error(msg string) {
	fmt.Fprintf(l.out, "%s %s\n", prefixWarden, color.RedString("ERROR: %s", msg))
}

// ErrorWithFix prints a failure with a known cause and the remedy for it.
// It is not a bug, so there is no issue link.
func (l *Logger) ErrorWithFix(err error, fix string) {
	l.Error(err.Error())
	if fix != "" {
		fmt.Fprintf(l.out, "  To fix: %s\n", fix)
	}
}

// ErrorWithIssueLink prints an error with a link to file a GitHub issue.
func (l *Logger) ErrorWithIssueLink(err error) {
	l.Error(err.Error())

	title := url.QueryEscape(fmt.Sprintf("Bug: %s", err.Error()))
	body := url.QueryEscape(fmt.Sprintf(
		"## Environment\n- OS: %s/%s\n- Version: %s\n"+
			"\n## Error\n```\n%s\n```\n"+
			"\n## Steps to reproduce\n1. \n",
		runtime.GOOS, runtime.GOARCH, version, err.Error(),
	))
	link := fmt.Sprintf(
		"https://github.com/buildwarden/buildwarden/issues/new"+
			"?title=%s&body=%s", title, body)

	fmt.Fprintf(l.out,
		"\n  If this is unexpected, please file a bug:\n  %s\n\n", link)
}

func logError(err error) {
	log.ErrorWithIssueLink(err)
}

func setColorMode(mode string) {
	switch mode {
	case "never":
		color.NoColor = true
	case "always":
		color.NoColor = false
	}
}

