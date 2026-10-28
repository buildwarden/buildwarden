package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The baseline is what initialize sets up in every build environment, from the
// relay CA alone: trust-store and build-system integrations, and best-practice
// defaults. It is not configurable (BOOT-05). A build sets anything else in its
// own script, whose ENV lines override these.

// jksAlias is the warden CA's alias in the Java truststore.
const jksAlias = "warden"

// bundleEnv are the variables tools read for an explicit CA bundle path.
var bundleEnv = []string{
	"SSL_CERT_FILE",
	"REQUESTS_CA_BUNDLE",
	"CURL_CA_BUNDLE",
	"NODE_EXTRA_CA_CERTS",
	"PIP_CERT",
	"CARGO_HTTP_CAINFO",
	"NIX_SSL_CERT_FILE",
	"HEX_CACERTS_PATH",
}

// applyBaseline writes the Java truststore and the Bazel system rc, and returns
// the baseline environment. bundle is the system CA bundle path ("" if none).
func applyBaseline(ca *caCert, bundle string) (map[string]string, error) {
	env := map[string]string{
		"SOURCE_DATE_EPOCH": "0",
		"UV_NATIVE_TLS":     "1",
	}
	if bundle != "" {
		for _, k := range bundleEnv {
			env[k] = bundle
		}
	}

	jks, err := writeJKS(jksAlias, ca.cert, jksPassword)
	if err != nil {
		return nil, err
	}
	jksPath := filepath.Join(wardenDataDir(), "certs.jks")
	if err := writeFileMkdir(jksPath, jks, 0o644); err != nil {
		return nil, fmt.Errorf("writing Java truststore: %w", err)
	}
	trust := filepath.ToSlash(jksPath)
	jvm := "-Djavax.net.ssl.trustStore=" + trust +
		" -Djavax.net.ssl.trustStorePassword=" + jksPassword
	env["MAVEN_OPTS"] = jvm
	env["GRADLE_OPTS"] = jvm

	rc := "startup --host_jvm_args=-Djavax.net.ssl.trustStore=" + trust +
		" --host_jvm_args=-Djavax.net.ssl.trustStorePassword=" + jksPassword + "\n"
	if err := writeFileMkdir(bazelSystemRC(), []byte(rc), 0o644); err != nil {
		return nil, fmt.Errorf("writing bazelrc: %w", err)
	}
	return env, nil
}

func writeFileMkdir(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

// shellEnvScript renders env as POSIX sh exports, sorted, plus a cd into
// workdir, for an interactive shell to source (initialize --hold).
func shellEnvScript(env map[string]string, workdir string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "export %s=%s\n", k, shQuote(env[k]))
	}
	if workdir != "" {
		fmt.Fprintf(&b, "cd %s\n", shQuote(workdir))
	}
	return b.String()
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
