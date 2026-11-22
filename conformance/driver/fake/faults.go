package main

import "os"

// The faults WARDEN_FAKE_FAULT may name, each a way a driver can break the
// contract. The suite's self-test checks that the check of each fails under
// it; the last five are the lifecycle tier's.
const (
	faultStrayStdout       = "stray-stdout"
	faultWrongExit         = "wrong-exit-code"
	faultUndeclaredCommand = "undeclared-command"
	faultAcceptArg         = "accept-undeclared-argument"
	faultCwdConfig         = "read-cwd-warden-toml"
	faultRefuseUnowned     = "refuse-unowned-table"
	faultUnlistedCommon    = "read-unlisted-common-key"
	faultWardenConfig      = "read-warden-config"
	faultDriverFirst       = "driver-before-own-table"
	faultWrongType         = "accept-wrong-type"
	faultReapConfigGone    = "reap-fails-config-gone"
	faultAcceptCase        = "accept-another-case"
	faultNoTeardown        = "no-teardown"
	faultIgnoreEOF         = "ignore-end-of-input"
	faultDieClosedStderr   = "die-on-closed-stderr"
	faultReapNoLook        = "reap-without-looking"
	// faultOrphanAlive: run leaves a child that names the build, which
	// outlives a killed driver, and clean counts that child as a sign that
	// the build lives, as the container driver's scan once counted any
	// process that named a build.
	faultOrphanAlive = "orphan-counts-as-alive"
)

// fault is the fault this process plays, or "".
var fault = os.Getenv("WARDEN_FAKE_FAULT")
