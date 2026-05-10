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

var (
	prefixWarden = color.New(color.FgCyan).Sprint("[warden]")
	prefixRelay  = color.New(color.FgMagenta).Sprint("[relay] ")
	prefixBuild  = color.New(color.FgBlue).Sprint("[build] ")
)

func (l *Logger) Info(msg string) {
	fmt.Fprintf(l.out, "%s %s\n", prefixWarden, msg)
}

func (l *Logger) Success(msg string) {
	fmt.Fprintf(l.out, "%s %s\n", prefixWarden, color.GreenString(msg))
}

func (l *Logger) Warn(msg string) {
	fmt.Fprintf(l.out, "%s %s\n", prefixWarden, color.YellowString(msg))
}

func (l *Logger) Error(msg string) {
	fmt.Fprintf(l.out, "%s %s\n", prefixWarden, color.RedString("ERROR: %s", msg))
}

func (l *Logger) Relay(msg string) {
	fmt.Fprintf(l.out, "%s %s\n", prefixRelay, msg)
}

func (l *Logger) Build(msg string) {
	fmt.Fprintf(l.out, "%s %s\n", prefixBuild, msg)
}

func (l *Logger) Result(msg string) {
	style := color.New(color.FgYellow, color.Bold)
	fmt.Fprintf(l.out, "%s %s\n", prefixWarden, style.Sprint(msg))
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

