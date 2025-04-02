package main

import (
	"testing"
)

func TestSetupLog(t *testing.T) {
	setupLog(true)
	setupLog(false)
	setupLog(true, "secret1", "secret2")
}