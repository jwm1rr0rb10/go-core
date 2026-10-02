package tcp

import (
	"crypto/tls"
)

// ServerTLSConfig loads a certificate/key pair and returns a server config
// with TLS 1.2 as the minimum version.
func ServerTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, wrapError("load key pair", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// ClientTLSConfig returns a client config with TLS 1.2 as the minimum
// version. insecureSkipVerify disables certificate verification and must only
// be used in tests.
func ClientTLSConfig(insecureSkipVerify bool) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // opt-in, documented as test-only
		MinVersion:         tls.VersionTLS12,
	}
}
