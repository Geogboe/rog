package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunVersionPrintsResolvedVersion(t *testing.T) {
	originalVersion := version
	version = "0.2.0-test"
	t.Cleanup(func() {
		version = originalVersion
	})

	output := captureOutput(func() {
		runVersion(nil, nil)
	})

	assert.Equal(t, "0.2.0-test\n", output)
}

func TestVersionCommandRejectsArgs(t *testing.T) {
	err := versionCmd.Args(versionCmd, []string{"extra"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command \"extra\" for \"rog version\"")
}
