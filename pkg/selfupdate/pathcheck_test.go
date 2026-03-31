package selfupdate

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sep is the OS path list separator (":" on Unix, ";" on Windows).
const sep = string(os.PathListSeparator)

func TestIsInPath_Found(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PATH", tmpDir+sep+"/other/dir")
	assert.True(t, IsInPath(tmpDir))
}

func TestIsInPath_NotFound(t *testing.T) {
	t.Setenv("PATH", "/some/other/dir"+sep+"/another/dir")
	assert.False(t, IsInPath("/not/in/path"))
}

func TestIsInPath_Empty(t *testing.T) {
	t.Setenv("PATH", "")
	assert.False(t, IsInPath("/usr/local/bin"))
}

func TestPathWarningMessage_InPath_ReturnsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PATH", tmpDir)
	assert.Empty(t, PathWarningMessage(tmpDir))
}

func TestPathWarningMessage_NotInPath_ContainsDir(t *testing.T) {
	t.Setenv("PATH", "/some/other/dir")
	msg := PathWarningMessage("/my/install/dir")
	require.NotEmpty(t, msg)
	assert.Contains(t, msg, "/my/install/dir")
}

func TestPathWarningMessage_NotInPath_ContainsInstructions(t *testing.T) {
	t.Setenv("PATH", "/some/other/dir")
	// Unset SHELL so we get the generic fallback on Unix
	t.Setenv("SHELL", "")
	msg := PathWarningMessage("/my/install/dir")
	require.NotEmpty(t, msg)
	// Should contain some kind of instruction
	assert.True(t,
		strings.Contains(msg, "PATH") || strings.Contains(msg, "path"),
		"message should mention PATH",
	)
}

func TestPathWarningMessage_TrailingSlash(t *testing.T) {
	tmpDir := t.TempDir()
	// Add with trailing separator; cleaned path should still match
	t.Setenv("PATH", tmpDir+string(os.PathSeparator)+sep+"/other")
	assert.True(t, IsInPath(tmpDir))
}
