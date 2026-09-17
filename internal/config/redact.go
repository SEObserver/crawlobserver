package config

import (
	"encoding/json"
	"strings"
)

// SessionConfigJSON returns the non-sensitive crawl config that is safe to store
// with a crawl session and send back through session APIs.
func SessionConfigJSON(cfg *Config) (string, error) {
	if cfg == nil {
		return "", nil
	}
	snapshot := struct {
		Crawler CrawlerConfig
	}{
		Crawler: cfg.Crawler,
	}
	// Request headers are dropped rather than recorded. They are free text a
	// person typed, so they can hold a credential that the key-name rules
	// below would not recognise, and the snapshot has no use for them: a crawl
	// that resumes reads its project's headers again rather than replaying
	// these, which would be expired by then anyway.
	snapshot.Crawler.Headers = nil
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return RedactSensitiveConfigJSON(string(data)), nil
}

// RedactSensitiveConfigJSON removes secret-like fields from a JSON config blob.
// Invalid or non-object config data is treated as unsafe and hidden.
func RedactSensitiveConfigJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return ""
	}
	redactSensitiveFields(data)

	safe, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(safe)
}

func redactSensitiveFields(value interface{}) {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, child := range v {
			if isSensitiveConfigKey(key) {
				delete(v, key)
				continue
			}
			redactSensitiveFields(child)
		}
	case []interface{}:
		for _, child := range v {
			redactSensitiveFields(child)
		}
	}
}

func isSensitiveConfigKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
	return strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "token")
}
