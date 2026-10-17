package git

import "strings"

// sanitizedGitEnv removes process-level Git config injection that would make
// git subprocess behavior depend on the caller environment instead of the repo
// or explicit command options.
func sanitizedGitEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, kv := range env {
		key := kv
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			key = kv[:idx]
		}
		if shouldDropGitEnv(key) {
			continue
		}
		filtered = append(filtered, kv)
	}
	return filtered
}

func shouldDropGitEnv(key string) bool {
	switch key {
	case "GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_SYSTEM":
		return true
	}

	return strings.HasPrefix(key, "GIT_CONFIG_KEY_") || strings.HasPrefix(key, "GIT_CONFIG_VALUE_")
}
