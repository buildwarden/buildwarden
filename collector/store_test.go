package collector

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A symlink in the build context that points outside it must never let the
// guest read the host file it names, the way `docker build` refuses it too.
// A symlink that stays inside the context still works.
func TestContextSymlinksCannotEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs privileges on Windows")
	}
	secretDir := t.TempDir()
	secret := filepath.Join(secretDir, "id_ed25519")
	if err := os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctxDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(ctxDir, "real.txt"), []byte("ok"), 0o644)
	mustSymlink(t, secret, filepath.Join(ctxDir, "key"))        // escapes
	mustSymlink(t, secretDir, filepath.Join(ctxDir, "secrets")) // escapes (dir)
	mustSymlink(t, "real.txt", filepath.Join(ctxDir, "alias"))  // stays inside

	srv, _ := newTestServer(t, Config{Token: "t", ContextDir: ctxDir})
	get := func(p string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/context/"+p, nil)
		req.Header.Set("Authorization", "Bearer t")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	for _, p := range []string{"key", "secrets/id_ed25519", "secrets/"} {
		code, body := get(p)
		if code == http.StatusOK || strings.Contains(body, "PRIVATE") {
			t.Errorf("GET %s escaped the context: %d %q", p, code, body)
		}
	}
	if code, body := get("alias"); code != http.StatusOK || body != "ok" {
		t.Errorf("in-context symlink: %d %q, want 200 \"ok\"", code, body)
	}
	if _, list := get(""); list != "alias\nreal.txt\n" {
		t.Errorf("listing = %q, want only the files reachable inside the context", list)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
