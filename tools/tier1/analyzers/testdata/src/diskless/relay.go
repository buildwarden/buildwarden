package diskless

import (
	"io/ioutil"
	"os"
)

func bad() {
	_, _ = os.Create("x")             // want `os.Create writes to disk`
	_ = os.WriteFile("x", nil, 0o600) // want `os.WriteFile writes to disk`
	_ = os.MkdirAll("d", 0o700)       // want `os.MkdirAll writes to disk`
	_ = ioutil.WriteFile("x", nil, 0) // want `ioutil.WriteFile writes to disk`
}

func fine() {
	_, _ = os.ReadFile("x")
	_ = os.Getenv("HOME")
}
