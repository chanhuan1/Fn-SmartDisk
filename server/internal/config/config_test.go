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

func TestListenAddrDoesNotImplyPackagedPort(t *testing.T) {
	if got := (Config{}).ListenAddr(); got != "127.0.0.1:0" {
		t.Fatalf("default development address=%q want 127.0.0.1:0", got)
	}
	if got := (Config{HTTPAddr: "127.0.0.1:19000"}).ListenAddr(); got != "127.0.0.1:19000" {
		t.Fatalf("explicit HTTPAddr=%q", got)
	}
	if got := (Config{Port: 19001}).ListenAddr(); got != "0.0.0.0:19001" {
		t.Fatalf("explicit Port=%q", got)
	}
}
