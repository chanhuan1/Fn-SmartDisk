package config

import "testing"

func TestGatewaySocketMode(t *testing.T) {
	t.Setenv("SMARTDISK_GATEWAY_SOCKET_MODE", "0660")
	if got := Load().GatewaySocketMode; got != 0o660 {
		t.Fatalf("socket mode=%#o want 0660", got)
	}
	t.Setenv("SMARTDISK_GATEWAY_SOCKET_MODE", "invalid")
	if got := Load().GatewaySocketMode; got != 0o600 {
		t.Fatalf("invalid socket mode=%#o want default 0600", got)
	}
}
