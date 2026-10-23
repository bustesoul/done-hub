package test

import (
	"os"
	"testing"

	"done-hub/common/logger"
	"done-hub/common/requester"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// RequireExternalIntegration explicitly opts in to real sends/uploads. Keep
// credentials local to this test instead of changing the application's config.
func RequireExternalIntegration(t testing.TB, requiredKeys ...string) *viper.Viper {
	t.Helper()
	if os.Getenv("DONE_HUB_RUN_EXTERNAL_TESTS") != "1" {
		t.Skip("real service test: set DONE_HUB_RUN_EXTERNAL_TESTS=1 and DONE_HUB_TEST_CONFIG to opt in")
	}
	path := os.Getenv("DONE_HUB_TEST_CONFIG")
	if path == "" {
		t.Fatal("DONE_HUB_TEST_CONFIG must point to an integration test config file")
	}
	cfg := viper.New()
	cfg.SetConfigFile(path)
	if err := cfg.ReadInConfig(); err != nil {
		t.Fatalf("read integration test config: %v", err)
	}
	for _, key := range requiredKeys {
		if cfg.GetString(key) == "" {
			t.Skipf("integration test config is missing %s", key)
		}
	}
	oldLogger, oldClient := logger.Logger, requester.HTTPClient
	logger.Logger = zap.NewNop()
	requester.InitHttpClient()
	t.Cleanup(func() {
		requester.HTTPClient.CloseIdleConnections()
		logger.Logger, requester.HTTPClient = oldLogger, oldClient
	})
	return cfg
}
