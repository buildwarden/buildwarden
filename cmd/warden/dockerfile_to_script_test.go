package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDockerfile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDockerfileToScript_SimpleRun(t *testing.T) {
	path := writeDockerfile(t, `FROM python:3.12-slim
RUN pip install requests
RUN echo hello
`)
	result, err := dockerfileToScript(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Image != "python:3.12-slim" {
		t.Errorf("image = %q, want python:3.12-slim", result.Image)
	}
	if !strings.Contains(result.Script, "pip install requests") {
		t.Error("script should contain pip install")
	}
	if !strings.Contains(result.Script, "echo hello") {
		t.Error("script should contain echo hello")
	}
	if !strings.HasPrefix(result.Script, "#!/bin/sh\nset -e\n") {
		t.Error("script should have shebang and set -e")
	}
}

func TestDockerfileToScript_ENV(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
ENV MY_VAR=hello
ENV MULTI=one TWO=2
RUN echo $MY_VAR
`)
	result, err := dockerfileToScript(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Script, "export MY_VAR=hello") {
		t.Errorf("script should export MY_VAR, got:\n%s", result.Script)
	}
	if !strings.Contains(result.Script, "export MULTI=one") {
		t.Errorf("script should export MULTI, got:\n%s", result.Script)
	}
}

func TestDockerfileToScript_WORKDIR(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
WORKDIR /app
RUN pwd
`)
	result, err := dockerfileToScript(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Script, "mkdir -p /app\ncd /app") {
		t.Errorf("WORKDIR should become mkdir+cd, got:\n%s", result.Script)
	}
}

func TestDockerfileToScript_ARG(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
ARG VERSION=1.0
RUN echo $VERSION
`)
	result, err := dockerfileToScript(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Script, ": ${VERSION:=1.0}") {
		t.Errorf("ARG should use shell default, got:\n%s", result.Script)
	}
}

func TestDockerfileToScript_MultiStage_Rejected(t *testing.T) {
	path := writeDockerfile(t, `FROM golang:1.22 AS builder
RUN go build .
FROM alpine
COPY --from=builder /app /app
`)
	_, err := dockerfileToScript(path)
	if err == nil {
		t.Fatal("expected error for multi-stage build")
	}
	if !strings.Contains(err.Error(), "multi-stage") {
		t.Errorf("error should mention multi-stage, got: %v", err)
	}
}

func TestDockerfileToScript_LineContinuation(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
RUN apt-get update && \
    apt-get install -y git && \
    rm -rf /var/lib/apt/lists/*
`)
	result, err := dockerfileToScript(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Script, "apt-get update") {
		t.Error("continuation should be joined")
	}
	if !strings.Contains(result.Script, "rm -rf") {
		t.Error("full continued command should be present")
	}
}

func TestDockerfileToScript_Comments(t *testing.T) {
	path := writeDockerfile(t, `# This is a comment
FROM alpine
# Another comment
RUN echo works
`)
	result, err := dockerfileToScript(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Script, "comment") {
		t.Error("comments should be stripped")
	}
	if !strings.Contains(result.Script, "echo works") {
		t.Error("RUN should still work")
	}
}

func TestDockerfileToScript_MetadataDirectivesIgnored(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
EXPOSE 8080
LABEL version="1.0"
CMD ["sh"]
ENTRYPOINT ["/bin/sh"]
RUN echo built
`)
	result, err := dockerfileToScript(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Script, "echo built") {
		t.Error("RUN should be present")
	}
}

func TestDockerfileToScript_NoFrom(t *testing.T) {
	path := writeDockerfile(t, `RUN echo oops
`)
	_, err := dockerfileToScript(path)
	if err == nil {
		t.Fatal("expected error for missing FROM")
	}
}

func TestDockerfileToScript_USERRejected(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
USER nobody
RUN whoami
`)
	_, err := dockerfileToScript(path)
	if err == nil {
		t.Fatal("expected error for USER directive")
	}
	if !strings.Contains(err.Error(), "USER") {
		t.Errorf("error should mention USER, got: %v", err)
	}
}

func TestDockerfileToScript_RealWorldSimple(t *testing.T) {
	path := writeDockerfile(t, `FROM python:3.12-slim

RUN pip install --upgrade pip build

RUN pip download --no-binary :all: --no-deps requests==2.32.3 -d /tmp/sdist && \
    tar xzf /tmp/sdist/requests-*.tar.gz -C /tmp && \
    cd /tmp/requests-*/ && \
    python -m build --wheel && \
    cp dist/requests-*.whl /tmp/

RUN WHEEL=$(ls /tmp/requests-*.whl) && \
    warden-io post "$WHEEL"
`)
	result, err := dockerfileToScript(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Image != "python:3.12-slim" {
		t.Errorf("image = %q", result.Image)
	}
	if !strings.Contains(result.Script, "warden-io post") {
		t.Error("script should contain artifact post")
	}
}
