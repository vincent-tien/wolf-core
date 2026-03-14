package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidYAML_Succeeds(t *testing.T) {
	cfg, err := Load("testdata/config.yaml")

	require.NoError(t, err)
	assert.Equal(t, "wolf-prime", cfg.App.Name)
	assert.Equal(t, EnvDevelopment, cfg.App.Env)
	assert.Equal(t, 8080, cfg.HTTP.Port)
	assert.NotEmpty(t, cfg.DB.Write.DSN)
}

func TestLoad_MissingFile_FallsBackToDefaults(t *testing.T) {
	// Without env overrides for critical fields (DSN, JWT), validation fails.
	// This proves the env-only path is reached and defaults are populated.
	_, err := Load("/tmp/nonexistent-wolf-config-test.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.write.dsn must not be empty")
}

func TestLoad_MissingFile_WithEnvOverrides_Succeeds(t *testing.T) {
	setTestEnv(t, map[string]string{
		"DB_WRITE_DSN":   "postgres://u:p@host:5432/db?sslmode=disable",
		"DB_READ_DSN":    "postgres://u:p@host:5432/db?sslmode=disable",
		"JWT_SECRET_KEY": "test-secret-key-that-is-at-least-32-chars-long!!",
	})

	cfg, err := Load("/tmp/nonexistent-wolf-config-test.yaml")

	require.NoError(t, err)
	assert.Equal(t, "wolf-prime", cfg.App.Name)
	assert.Equal(t, 8080, cfg.HTTP.Port)
	assert.Equal(t, "postgres://u:p@host:5432/db?sslmode=disable", cfg.DB.Write.DSN)
	assert.Equal(t, "postgres://u:p@host:5432/db?sslmode=disable", cfg.DB.Read.DSN)
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	t.Setenv("APP_NAME", "overridden-name")
	t.Setenv("LOG_LEVEL", "warn")

	cfg, err := Load("testdata/config.yaml")

	require.NoError(t, err)
	assert.Equal(t, "overridden-name", cfg.App.Name)
	assert.Equal(t, "warn", cfg.Log.Level)
}

func TestLoad_MalformedYAML_ReturnsError(t *testing.T) {
	tmp := writeTempFile(t, "malformed.yaml", []byte("{{invalid yaml"))

	_, err := Load(tmp)

	require.Error(t, err)
}

func TestLoad_PermissionDenied_ReturnsError(t *testing.T) {
	tmp := writeTempFile(t, "unreadable.yaml", []byte("app:\n  name: test"))
	require.NoError(t, os.Chmod(tmp, 0o000))
	t.Cleanup(func() { os.Chmod(tmp, 0o644) })

	_, err := Load(tmp)

	require.Error(t, err)
}

func TestLoad_ValidationFailure_ReturnsError(t *testing.T) {
	// Valid YAML but invalid config values.
	content := []byte(`
app:
  name: test
  env: invalid-env
http:
  port: 8080
  read_timeout: 10s
  write_timeout: 15s
  idle_timeout: 120s
  max_header_bytes: 1048576
`)
	tmp := writeTempFile(t, "bad-values.yaml", content)

	_, err := Load(tmp)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "app.env")
}

func TestLoad_EnvOnly_PopulatesAllCriticalFields(t *testing.T) {
	setTestEnv(t, map[string]string{
		"APP_NAME":       "env-app",
		"APP_ENV":        "staging",
		"HTTP_PORT":      "9000",
		"GRPC_PORT":      "9091",
		"METRICS_PORT":   "9092",
		"DB_WRITE_DSN":   "postgres://w:w@db:5432/wolf?sslmode=disable",
		"DB_READ_DSN":    "postgres://r:r@db:5432/wolf?sslmode=disable",
		"CACHE_DRIVER":   "redis",
		"REDIS_ADDR":     "redis:6379",
		"BROKER_DRIVER":  "nats",
		"NATS_URL":       "nats://nats:4222",
		"NATS_STREAM":    "wolf",
		"LOG_LEVEL":      "info",
		"JWT_SECRET_KEY": "production-secret-key-that-is-at-least-32-chars!!",
		"JWT_ISSUER":     "wolf-prime",
	})

	cfg, err := Load("/tmp/nonexistent-wolf-config-test.yaml")

	require.NoError(t, err)
	assert.Equal(t, "env-app", cfg.App.Name)
	assert.Equal(t, EnvStaging, cfg.App.Env)
	assert.Equal(t, 9000, cfg.HTTP.Port)
	assert.Equal(t, "postgres://w:w@db:5432/wolf?sslmode=disable", cfg.DB.Write.DSN)
	assert.Equal(t, "nats", cfg.Broker.Driver)
	assert.Equal(t, "nats://nats:4222", cfg.NATS.URL)
}

// setTestEnv sets multiple env vars for the test's duration.
func setTestEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

// writeTempFile creates a temporary file and returns its path.
func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}
