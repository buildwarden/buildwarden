package driver

import (
	"fmt"
	"os"
	"testing"
)

// detachedHelperEnv, when set, makes the test binary a stand-in for warden in
// the detached-start tests: it prints the file it was started from and exits.
const detachedHelperEnv = "WARDEN_TEST_DETACHED_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(detachedHelperEnv) != "" {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "os.Executable: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(exe)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
