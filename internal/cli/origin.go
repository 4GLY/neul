package cli

import (
	"errors"
	"net/url"
	"strings"
)

func normalizeServerOrigin(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("login requires --server")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("login requires --server <origin>")
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}
