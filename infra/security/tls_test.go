package security_test

import (
	"crypto/tls"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vincent-tien/wolf-core/infra/security"
)

func TestServerTLSConfig_InvalidCertReturnsError(t *testing.T) {
	_, err := security.ServerTLSConfig("/nonexistent/cert.pem", "/nonexistent/key.pem")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load TLS cert")
}

func TestClientTLSConfig_InvalidCAReturnsError(t *testing.T) {
	_, err := security.ClientTLSConfig("/nonexistent/ca.pem")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read CA cert")
}

func TestClientTLSConfig_InvalidPEMReturnsError(t *testing.T) {
	tmpFile := t.TempDir() + "/bad-ca.pem"
	err := os.WriteFile(tmpFile, []byte("not a PEM"), 0o600)
	assert.NoError(t, err)

	_, err = security.ClientTLSConfig(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse CA certificate")
}

func TestMutualTLSConfig_InvalidCertReturnsError(t *testing.T) {
	_, err := security.MutualTLSConfig("/bad/cert.pem", "/bad/key.pem", "/bad/ca.pem")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load mTLS cert")
}

func TestTLSVersionConstant(t *testing.T) {
	assert.Equal(t, uint16(0x0303), uint16(tls.VersionTLS12),
		"TLS 1.2 constant must be 0x0303")
}
