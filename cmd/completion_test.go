package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to execute a command and capture output
func executeCompletionCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)

	// Temporarily disable os.Exit for tests
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	err := rootCmd.Execute()

	// Reset args for next test
	rootCmd.SetArgs([]string{})

	return buf.String(), err
}

func TestCompletionCommandExists(t *testing.T) {
	output, err := executeCompletionCommand(t, "completion", "--help")
	require.NoError(t, err)

	assert.Contains(t, output, "completion")
	assert.Contains(t, output, "autocompletion")
}

func TestCompletionAllShellsSupported(t *testing.T) {
	output, err := executeCompletionCommand(t, "completion", "--help")
	require.NoError(t, err)

	// Check that all expected shell subcommands are mentioned
	expectedShells := []string{"bash", "zsh", "fish", "powershell"}
	for _, shell := range expectedShells {
		assert.Contains(t, output, shell, "completion should support %s", shell)
	}
}

func TestCompletionBashHelp(t *testing.T) {
	output, err := executeCompletionCommand(t, "completion", "bash", "--help")
	require.NoError(t, err)

	// Verify help contains bash-specific instructions
	assert.Contains(t, output, "bash")
	assert.Contains(t, output, "autocompletion")
}

func TestCompletionZshHelp(t *testing.T) {
	output, err := executeCompletionCommand(t, "completion", "zsh", "--help")
	require.NoError(t, err)

	// Verify help contains zsh-specific instructions
	assert.Contains(t, output, "zsh")
	assert.Contains(t, output, "autocompletion")
}

func TestCompletionFishHelp(t *testing.T) {
	output, err := executeCompletionCommand(t, "completion", "fish", "--help")
	require.NoError(t, err)

	// Verify help contains fish-specific instructions
	assert.Contains(t, output, "fish")
	assert.Contains(t, output, "autocompletion")
}

func TestCompletionPowershellHelp(t *testing.T) {
	output, err := executeCompletionCommand(t, "completion", "powershell", "--help")
	require.NoError(t, err)

	// Verify help contains powershell-specific instructions
	assert.Contains(t, output, "powershell")
	assert.Contains(t, output, "autocompletion")
}

// Integration test: Verify completion commands execute without error
func TestCompletionBashExecutes(t *testing.T) {
	_, err := executeCompletionCommand(t, "completion", "bash")
	// Completion command should execute without error
	// We can't capture output as it writes directly to os.Stdout
	require.NoError(t, err)
}

func TestCompletionZshExecutes(t *testing.T) {
	_, err := executeCompletionCommand(t, "completion", "zsh")
	require.NoError(t, err)
}

func TestCompletionFishExecutes(t *testing.T) {
	_, err := executeCompletionCommand(t, "completion", "fish")
	require.NoError(t, err)
}

func TestCompletionPowershellExecutes(t *testing.T) {
	_, err := executeCompletionCommand(t, "completion", "powershell")
	require.NoError(t, err)
}
