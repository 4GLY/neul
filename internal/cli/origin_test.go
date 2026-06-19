package cli

import "testing"

func TestLoginNormalizeServerOrigin_whenOriginHasTrailingSlash_returnsCanonicalOrigin(t *testing.T) {
	got, err := normalizeServerOrigin("HTTPS://Neul.Example:8443/")
	if err != nil {
		t.Fatalf("normalizeServerOrigin() error = %v", err)
	}
	if got != "https://neul.example:8443" {
		t.Fatalf("origin = %q, want canonical origin", got)
	}
}

func TestLoginNormalizeServerOrigin_whenNonOriginURL_rejects(t *testing.T) {
	tests := []string{
		"https://owner:token@neul.example",
		"https://neul.example/path",
		"https://neul.example/?token=secret",
		"https://neul.example/#fragment",
		"https://neul.example/path?token=secret#fragment",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if got, err := normalizeServerOrigin(raw); err == nil {
				t.Fatalf("normalizeServerOrigin() = %q, nil error; want rejection", got)
			}
		})
	}
}
