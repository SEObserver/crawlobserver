package schema

import (
	"encoding/json"
	"strings"
	"time"
)

// ValidationIssue represents a single validation error or warning.
type ValidationIssue struct {
	Level   ValidationLevel `json:"level"`
	Message string          `json:"message"`
}

// ValidationResult holds the validation outcome for one structured data item.
type ValidationResult struct {
	SchemaType string            `json:"schema_type"`
	RawJSON    string            `json:"raw_json"`
	Errors     []ValidationIssue `json:"errors"`
	Warnings   []ValidationIssue `json:"warnings"`
	IsValid    bool              `json:"is_valid"`
}

// StructuredDataItem is the storage-ready form of a validated structured data block.
type StructuredDataItem struct {
	CrawlSessionID string
	URL            string
	SchemaType     string
	JSONLD         string
	Errors         []string
	Warnings       []string
	IsValid        bool
	Source         string // "static" or "rendered"
	CrawledAt      time.Time
}

// ValidateBlock parses a single JSON-LD block and validates all items within it.
func ValidateBlock(jsonLD string) []ValidationResult {
	jsonLD = strings.TrimSpace(jsonLD)
	if jsonLD == "" {
		return nil
	}

	// Try parsing as a single object first
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonLD), &obj); err == nil {
		return validateObject(obj, jsonLD)
	}

	// Try parsing as an array of objects
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonLD), &arr); err == nil {
		var results []ValidationResult
		for _, item := range arr {
			itemJSON, _ := json.Marshal(item)
			results = append(results, validateObject(item, string(itemJSON))...)
		}
		return results
	}

	return nil
}

func validateObject(obj map[string]interface{}, rawJSON string) []ValidationResult {
	// Handle @graph: array of items in a single JSON-LD block
	if graph, ok := obj["@graph"]; ok {
		if graphArr, ok := graph.([]interface{}); ok {
			var results []ValidationResult
			for _, item := range graphArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					itemJSON, _ := json.Marshal(itemMap)
					results = append(results, validateSingleItem(itemMap, string(itemJSON))...)
				}
			}
			return results
		}
	}

	return validateSingleItem(obj, rawJSON)
}

func validateSingleItem(obj map[string]interface{}, rawJSON string) []ValidationResult {
	schemaType := extractType(obj)
	if schemaType == "" {
		return nil
	}

	// Handle multiple types (e.g., ["Restaurant", "LocalBusiness"])
	types := splitTypes(schemaType)
	var results []ValidationResult
	for _, t := range types {
		result := validateType(t, obj, rawJSON)
		results = append(results, result)
	}
	return results
}

func extractType(obj map[string]interface{}) string {
	t, ok := obj["@type"]
	if !ok {
		return ""
	}
	switch v := t.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	}
	return ""
}

func splitTypes(t string) []string {
	if strings.Contains(t, ",") {
		parts := strings.Split(t, ",")
		var result []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	}
	return []string{t}
}

func validateType(schemaType string, obj map[string]interface{}, rawJSON string) ValidationResult {
	result := ValidationResult{
		SchemaType: schemaType,
		RawJSON:    rawJSON,
		IsValid:    true,
	}

	rules, ok := Rules[schemaType]
	if !ok {
		// Unknown type — no rules to apply, considered valid
		return result
	}

	for _, rule := range rules {
		if !fieldExists(obj, rule.Field) {
			issue := ValidationIssue{
				Level:   rule.Level,
				Message: rule.Message,
			}
			switch rule.Level {
			case LevelError:
				result.Errors = append(result.Errors, issue)
				result.IsValid = false
			case LevelWarning:
				result.Warnings = append(result.Warnings, issue)
			}
		}
	}

	return result
}

// fieldExists checks if a (possibly nested) field exists in the JSON object.
// Supports dot notation: "offers.price" checks obj["offers"]["price"]
// or obj["offers"][0]["price"] for arrays.
func fieldExists(obj map[string]interface{}, path string) bool {
	parts := strings.SplitN(path, ".", 2)
	key := parts[0]

	val, ok := obj[key]
	if !ok || val == nil {
		return false
	}

	// If no more nesting, field exists
	if len(parts) == 1 {
		return true
	}

	rest := parts[1]

	// Check nested object
	switch v := val.(type) {
	case map[string]interface{}:
		return fieldExists(v, rest)
	case []interface{}:
		// Check if any item in the array has the nested field
		for _, item := range v {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if fieldExists(itemMap, rest) {
					return true
				}
			}
		}
	}

	return false
}

// maxJSONLDSize is the maximum size of a JSON-LD block stored in ClickHouse.
// Blocks larger than this are truncated to avoid storage bloat from malformed pages.
const maxJSONLDSize = 512 * 1024 // 512 KB

// ValidateAllBlocks validates multiple JSON-LD blocks and returns storage-ready items.
func ValidateAllBlocks(blocks []string, sessionID, url string, crawledAt time.Time, source string) []StructuredDataItem {
	var items []StructuredDataItem
	for _, block := range blocks {
		results := ValidateBlock(block)
		for _, r := range results {
			var errs []string
			for _, e := range r.Errors {
				errs = append(errs, e.Message)
			}
			var warns []string
			for _, w := range r.Warnings {
				warns = append(warns, w.Message)
			}
			jsonLD := r.RawJSON
			if len(jsonLD) > maxJSONLDSize {
				jsonLD = jsonLD[:maxJSONLDSize]
			}
			items = append(items, StructuredDataItem{
				CrawlSessionID: sessionID,
				URL:            url,
				SchemaType:     r.SchemaType,
				JSONLD:         jsonLD,
				Errors:         errs,
				Warnings:       warns,
				IsValid:        r.IsValid,
				Source:          source,
				CrawledAt:      crawledAt,
			})
		}
	}
	return items
}

// CountSummary returns (validCount, errorCount, warningCount) from items.
func CountSummary(items []StructuredDataItem) (valid, errors, warnings uint16) {
	for _, item := range items {
		if item.IsValid {
			valid++
		}
		if len(item.Errors) > 0 {
			errors++
		}
		if len(item.Warnings) > 0 {
			warnings++
		}
	}
	return
}
