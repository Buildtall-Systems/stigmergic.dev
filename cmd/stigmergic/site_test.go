package main

import (
	"slices"
	"testing"
)

func TestSiteCmdRejectsPositionalArgs(t *testing.T) {
	t.Parallel()

	if err := siteCmd.Args(siteCmd, []string{"./example"}); err == nil {
		t.Error("expected site command to reject positional arguments")
	}
}

func TestSiteCmdRegistered(t *testing.T) {
	t.Parallel()

	if !slices.Contains(rootCmd.Commands(), siteCmd) {
		t.Error("expected site command to be registered on the root command")
	}
}
