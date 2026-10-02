package tcp

import (
	"crypto/tls"
	"path/filepath"
	"testing"
)

func TestClientTLSConfig(t *testing.T) {
	cfg := ClientTLSConfig(true)
	if !cfg.InsecureSkipVerify || cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if ClientTLSConfig(false).InsecureSkipVerify {
		t.Fatal("verification must be on by default")
	}
}

func TestServerTLSConfig(t *testing.T) {
	_, _, certFile, keyFile := selfSigned(t, t.TempDir())
	cfg, err := ServerTLSConfig(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Certificates) != 1 || cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if _, err := ServerTLSConfig(filepath.Join(t.TempDir(), "missing"), keyFile); err == nil {
		t.Fatal("expected error for missing files")
	}
}
