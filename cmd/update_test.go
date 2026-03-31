package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureUpdateOutput runs fn and captures everything written to stdout+stderr.
func captureUpdateOutput(fn func()) string {
	return captureOutput(fn)
}

func TestUpdateCmd_CheckFlag_AlreadyUpToDate(t *testing.T) {
	// Set the current version so we can compare.
	orig := version
	version = "v0.5.0"
	t.Cleanup(func() { version = orig })

	// Override the updater factory to return a mock that reports the same version.
	origFactory := updateNewUpdater
	updateNewUpdater = func(opts updateOptions) updaterIface {
		return &mockUpdater{latestVersion: "v0.5.0"}
	}
	t.Cleanup(func() { updateNewUpdater = origFactory })

	out := captureUpdateOutput(func() {
		err := updateCmd.RunE(updateCmd, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, out, "Already up to date")
	assert.Contains(t, out, "v0.5.0")
}

func TestUpdateCmd_CheckFlag_UpdateAvailable(t *testing.T) {
	orig := version
	version = "v0.4.0"
	t.Cleanup(func() { version = orig })

	origFactory := updateNewUpdater
	updateNewUpdater = func(opts updateOptions) updaterIface {
		return &mockUpdater{latestVersion: "v0.5.0"}
	}
	t.Cleanup(func() { updateNewUpdater = origFactory })

	// Set --check flag
	require.NoError(t, updateCmd.Flags().Set("check", "true"))
	t.Cleanup(func() { updateCmd.Flags().Set("check", "false") })

	out := captureUpdateOutput(func() {
		err := updateCmd.RunE(updateCmd, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, out, "v0.4.0")
	assert.Contains(t, out, "v0.5.0")
	assert.Contains(t, out, "update available")
}

func TestUpdateCmd_Install_Success(t *testing.T) {
	orig := version
	version = "v0.4.0"
	t.Cleanup(func() { version = orig })

	origFactory := updateNewUpdater
	updateNewUpdater = func(opts updateOptions) updaterIface {
		return &mockUpdater{latestVersion: "v0.5.0"}
	}
	t.Cleanup(func() { updateNewUpdater = origFactory })

	// Provide a fake executable path via env so we don't try to replace ourselves.
	tmpExe, err := os.CreateTemp(t.TempDir(), "rog-test-*")
	require.NoError(t, err)
	tmpExe.Close()
	t.Setenv("ROG_TEST_EXE_PATH", tmpExe.Name())

	out := captureUpdateOutput(func() {
		err := updateCmd.RunE(updateCmd, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, out, "v0.5.0")
	assert.Contains(t, out, "updated")
}

func TestUpdateCmd_PinnedVersion(t *testing.T) {
	orig := version
	version = "v0.4.0"
	t.Cleanup(func() { version = orig })

	var capturedVersion string
	origFactory := updateNewUpdater
	updateNewUpdater = func(opts updateOptions) updaterIface {
		capturedVersion = opts.pinnedVersion
		return &mockUpdater{latestVersion: "v0.3.0"}
	}
	t.Cleanup(func() { updateNewUpdater = origFactory })

	require.NoError(t, updateCmd.Flags().Set("version", "v0.3.0"))
	t.Cleanup(func() { updateCmd.Flags().Set("version", "") })

	captureUpdateOutput(func() {
		updateCmd.RunE(updateCmd, nil)
	})

	assert.Equal(t, "v0.3.0", capturedVersion)
}

func TestUpdateCmd_TokenFromEnv(t *testing.T) {
	t.Setenv("ROG_GITHUB_TOKEN", "my-test-token")
	orig := version
	version = "v0.5.0"
	t.Cleanup(func() { version = orig })

	var capturedToken string
	origFactory := updateNewUpdater
	updateNewUpdater = func(opts updateOptions) updaterIface {
		capturedToken = opts.token
		return &mockUpdater{latestVersion: "v0.5.0"}
	}
	t.Cleanup(func() { updateNewUpdater = origFactory })

	captureUpdateOutput(func() {
		updateCmd.RunE(updateCmd, nil)
	})

	assert.Equal(t, "my-test-token", capturedToken)
}

func TestUpdateCmd_ProxyFromFlag(t *testing.T) {
	orig := version
	version = "v0.5.0"
	t.Cleanup(func() { version = orig })

	var capturedProxy string
	origFactory := updateNewUpdater
	updateNewUpdater = func(opts updateOptions) updaterIface {
		capturedProxy = opts.proxyURL
		return &mockUpdater{latestVersion: "v0.5.0"}
	}
	t.Cleanup(func() { updateNewUpdater = origFactory })

	require.NoError(t, updateCmd.Flags().Set("proxy", "http://proxy.example.com:8080"))
	t.Cleanup(func() { updateCmd.Flags().Set("proxy", "") })

	captureUpdateOutput(func() {
		updateCmd.RunE(updateCmd, nil)
	})

	assert.Equal(t, "http://proxy.example.com:8080", capturedProxy)
}

// mockUpdater implements updaterIface for testing.
type mockUpdater struct {
	latestVersion string
	installErr    error
}

func (m *mockUpdater) CheckLatest() (string, error) {
	return m.latestVersion, nil
}

func (m *mockUpdater) Install(version, exePath string) error {
	return m.installErr
}
