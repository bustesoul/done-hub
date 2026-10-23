package test

import (
	"os"
	"path/filepath"
	"testing"

	"done-hub/common/logger"
	"done-hub/common/requester"
	"github.com/stretchr/testify/require"
)

func TestExternalIntegrationRequiresOptIn(t *testing.T) {
	t.Setenv("DONE_HUB_RUN_EXTERNAL_TESTS", "")
	t.Setenv("DONE_HUB_TEST_CONFIG", "/must-not-be-read.yaml")
	oldLogger, oldClient := logger.Logger, requester.HTTPClient
	reachedService := false
	t.Run("disabled", func(t *testing.T) {
		RequireExternalIntegration(t)
		reachedService = true
	})
	require.False(t, reachedService)
	require.Same(t, oldLogger, logger.Logger)
	require.Same(t, oldClient, requester.HTTPClient)
}

func TestExternalIntegrationCredentialsAndCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"test":{"token":"local-only"}}`), 0600))
	t.Setenv("DONE_HUB_RUN_EXTERNAL_TESTS", "1")
	t.Setenv("DONE_HUB_TEST_CONFIG", path)
	oldLogger, oldClient := logger.Logger, requester.HTTPClient
	reachedService := false
	t.Run("missing credentials", func(t *testing.T) {
		RequireExternalIntegration(t, "missing.token")
		reachedService = true
	})
	require.False(t, reachedService)
	t.Run("configured without any network call", func(t *testing.T) {
		cfg := RequireExternalIntegration(t, "test.token")
		require.Equal(t, "local-only", cfg.GetString("test.token"))
		require.NotNil(t, logger.Logger)
		require.NotNil(t, requester.HTTPClient)
	})
	require.Same(t, oldLogger, logger.Logger)
	require.Same(t, oldClient, requester.HTTPClient)
}
