package config

import (
	"fmt"
	"os"
	"strings"
)

// ParseTargetsFromEnv parses the XUI_EXPORTER_TARGETS environment variable
// and returns a list of target URLs.
// Returns error if the environment variable is empty or contains no valid URLs.
func ParseTargetsFromEnv() ([]string, error) {
	env := os.Getenv("XUI_EXPORTER_TARGETS")
	if env == "" {
		return nil, fmt.Errorf("XUI_EXPORTER_TARGETS environment variable is required but not set")
	}

	// Split by comma and trim whitespace
	rawTargets := strings.Split(env, ",")
	var targets []string

	for _, target := range rawTargets {
		trimmed := strings.TrimSpace(target)
		if trimmed != "" {
			targets = append(targets, trimmed)
		}
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("XUI_EXPORTER_TARGETS contains no valid URLs after parsing")
	}

	return targets, nil
}
