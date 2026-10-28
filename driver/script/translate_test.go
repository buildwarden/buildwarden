package script

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

func TestTranslate_SimpleRun(t *testing.T) {
	path := writeDockerfile(t, `FROM python:3.12-slim
RUN pip install requests
RUN echo hello
`)
	result, err := Translate(path, "")
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

func TestTranslate_ENV(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
ENV MY_VAR=hello
ENV MULTI=one TWO=2
RUN echo $MY_VAR
`)
	result, err := Translate(path, "")
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

func TestTranslate_ENV_VarExpansion(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
ENV PATH="/root/.cargo/bin:$PATH"
`)
	result, err := Translate(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Script, `export PATH="/root/.cargo/bin:$PATH"`) {
		t.Errorf(
			"ENV with $VAR should use double quotes, got:\n%s",
			result.Script)
	}
}

func TestTranslate_WORKDIR(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
WORKDIR /app
RUN pwd
`)
	result, err := Translate(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Script, "mkdir -p /app\ncd /app") {
		t.Errorf("WORKDIR should become mkdir+cd, got:\n%s", result.Script)
	}
}

func TestTranslate_ARG(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
ARG VERSION=1.0
RUN echo $VERSION
`)
	result, err := Translate(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Script, ": ${VERSION:=1.0}") {
		t.Errorf("ARG should use shell default, got:\n%s", result.Script)
	}
}

func TestTranslate_MultiStage_Rejected(t *testing.T) {
	path := writeDockerfile(t, `FROM golang:1.22 AS builder
RUN go build .
FROM alpine
COPY --from=builder /app /app
`)
	_, err := Translate(path, "")
	if err == nil {
		t.Fatal("expected error for multi-stage build")
	}
	if !strings.Contains(err.Error(), "multi-stage") {
		t.Errorf("error should mention multi-stage, got: %v", err)
	}
}

func TestTranslate_LineContinuation(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
RUN apt-get update && \
    apt-get install -y git && \
    rm -rf /var/lib/apt/lists/*
`)
	result, err := Translate(path, "")
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

func TestTranslate_Comments(t *testing.T) {
	path := writeDockerfile(t, `# This is a comment
FROM alpine
# Another comment
RUN echo works
`)
	result, err := Translate(path, "")
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

func TestTranslate_MetadataDirectivesIgnored(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
EXPOSE 8080
LABEL version="1.0"
CMD ["sh"]
ENTRYPOINT ["/bin/sh"]
RUN echo built
`)
	result, err := Translate(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Script, "echo built") {
		t.Error("RUN should be present")
	}
}

func TestTranslate_NoFrom(t *testing.T) {
	path := writeDockerfile(t, `RUN echo oops
`)
	_, err := Translate(path, "")
	if err == nil {
		t.Fatal("expected error for missing FROM")
	}
}

func TestTranslate_USERRejected(t *testing.T) {
	path := writeDockerfile(t, `FROM alpine
USER nobody
RUN whoami
`)
	_, err := Translate(path, "")
	if err == nil {
		t.Fatal("expected error for USER directive")
	}
	if !strings.Contains(err.Error(), "USER") {
		t.Errorf("error should mention USER, got: %v", err)
	}
}

func TestTranslate_RealWorldSimple(t *testing.T) {
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
	result, err := Translate(path, "")
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

// translateIn writes files (path -> content) and a Containerfile into a fresh
// build context and translates it.
func translateIn(t *testing.T, files map[string]string, containerfile string) string {
	t.Helper()
	ctx := t.TempDir()
	for name, body := range files {
		p := filepath.Join(ctx, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cf := filepath.Join(ctx, "Containerfile")
	if err := os.WriteFile(cf, []byte(containerfile), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Translate(cf, ctx)
	if err != nil {
		t.Fatal(err)
	}
	return r.Script
}

// COPY sources are expanded on the host against the context: a file (or glob
// match) lands in a directory dest by its base name, as docker places it; a
// directory's files keep their path under it, even when it holds one file.
func TestTranslateCopyExpandsOnHost(t *testing.T) {
	files := map[string]string{
		"sub/deep/d.txt": "x", "sub/a.txt": "x", "sub/b.txt": "x", "one/in.txt": "x",
		".git/config": "x",
	}
	cases := map[string][]string{
		"COPY sub/deep/d.txt /opt/": {"warden-io fetch sub/deep/d.txt -o /opt/d.txt"},
		"COPY sub/*.txt /opt/": {
			"warden-io fetch sub/a.txt -o /opt/a.txt",
			"warden-io fetch sub/b.txt -o /opt/b.txt",
		},
		"COPY one /work/one": {"warden-io fetch one/in.txt -o /work/one/in.txt"},
		"COPY sub /data/": {
			"mkdir -p /data/deep",
			"warden-io fetch sub/deep/d.txt -o /data/deep/d.txt",
		},
		"COPY sub/a.txt /etc/app/conf": {
			"mkdir -p /etc/app", "warden-io fetch sub/a.txt -o /etc/app/conf",
		},
		"COPY --chmod=0755 sub/a.txt /bin/": {"chmod -R 0755 /bin/"},
		"COPY nothing* /x/":                 {"# (empty COPY: no matching files)"},
	}
	for line, want := range cases {
		got := translateIn(t, files, "FROM alpine\n"+line+"\n")
		for _, w := range want {
			if !strings.Contains(got, w+"\n") {
				t.Errorf("%s: want %q, got:\n%s", line, w, got)
			}
		}
	}
	got := translateIn(t, files, "FROM alpine\nCOPY . /src/\n")
	if strings.Contains(got, ".git") {
		t.Errorf("COPY . should skip .git, got:\n%s", got)
	}
}
