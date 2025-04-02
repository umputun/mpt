package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetupLog(t *testing.T) {
	// test different logging configurations
	setupLog(true)
	setupLog(false)
	setupLog(true, "secret1", "secret2")
}

func TestRun_VersionFlag(t *testing.T) {
	// save original osExit and restore it after the test
	oldOsExit := osExit
	defer func() { osExit = oldOsExit }()

	// mock os.Exit
	var exitCode int
	osExit = func(code int) {
		exitCode = code
		panic("os.Exit called")
	}

	// test the version flag
	os.Args = []string{"mpt", "--version"}

	// catch the panic from our mocked os.Exit
	defer func() {
		if r := recover(); r != nil {
			assert.Equal(t, "os.Exit called", r)
			assert.Equal(t, 0, exitCode)
		}
	}()

	run()
}