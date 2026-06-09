package main

import (
	"strings"
	"testing"
	"time"
)

func TestSetupTokenTTLFromEnv_whenEnvIsUnset_returnsZeroTTL(t *testing.T) {
	t.Setenv("NEUL_SETUP_TOKEN_TTL", "")

	ttl, err := setupTokenTTLFromEnv()

	if err != nil {
		t.Fatalf("setupTokenTTLFromEnv() error = %v, want nil", err)
	}
	if ttl != 0 {
		t.Fatalf("ttl = %v, want 0", ttl)
	}
}

func TestSetupTokenTTLFromEnv_whenEnvIsValid_returnsDuration(t *testing.T) {
	t.Setenv("NEUL_SETUP_TOKEN_TTL", "1ms")

	ttl, err := setupTokenTTLFromEnv()

	if err != nil {
		t.Fatalf("setupTokenTTLFromEnv() error = %v, want nil", err)
	}
	if ttl != time.Millisecond {
		t.Fatalf("ttl = %v, want %v", ttl, time.Millisecond)
	}
}

func TestSetupTokenTTLFromEnv_whenEnvIsMalformed_returnsError(t *testing.T) {
	t.Setenv("NEUL_SETUP_TOKEN_TTL", "not-a-duration")

	_, err := setupTokenTTLFromEnv()

	if err == nil {
		t.Fatal("setupTokenTTLFromEnv() error = nil, want error")
	}
}

func TestSetupTokenTTLFromEnv_whenEnvIsNonPositive_returnsError(t *testing.T) {
	for _, value := range []string{"0", "-1s"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("NEUL_SETUP_TOKEN_TTL", value)

			_, err := setupTokenTTLFromEnv()

			if err == nil {
				t.Fatal("setupTokenTTLFromEnv() error = nil, want error")
			}
			if !strings.Contains(err.Error(), "must be positive") {
				t.Fatalf("error = %q, want positive TTL message", err)
			}
		})
	}
}
