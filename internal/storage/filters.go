package storage

import (
	"fmt"
	"strconv"
	"strings"
)

type FilterType int

const (
	FilterLike  FilterType = iota // String → ILIKE '%val%'
	FilterUint                    // Numeric → =N, >N, <N, >=N, <=N
	FilterBool                    // Bool → = true/false
	FilterArray                   // Array(String) → arrayExists(x -> x ILIKE '%val%', col)
)

type FilterDef struct {
	Column string
	Type   FilterType
}

type ParsedFilter struct {
	Def   FilterDef
	Value string
}

// PageFilters defines the allowed filter columns for the pages table.
var PageFilters = map[string]FilterDef{
	"url":                {Column: "url", Type: FilterLike},
	"content_type":       {Column: "content_type", Type: FilterLike},
	"title":              {Column: "title", Type: FilterLike},
	"canonical":          {Column: "canonical", Type: FilterLike},
	"meta_robots":        {Column: "meta_robots", Type: FilterLike},
	"meta_description":   {Column: "meta_description", Type: FilterLike},
	"meta_keywords":      {Column: "meta_keywords", Type: FilterLike},
	"lang":               {Column: "lang", Type: FilterLike},
	"og_title":           {Column: "og_title", Type: FilterLike},
	"content_encoding":   {Column: "content_encoding", Type: FilterLike},
	"index_reason":       {Column: "index_reason", Type: FilterLike},
	"error":              {Column: "error", Type: FilterLike},
	"found_on":           {Column: "found_on", Type: FilterLike},
	"status_code":        {Column: "status_code", Type: FilterUint},
	"title_length":       {Column: "title_length", Type: FilterUint},
	"meta_desc_length":   {Column: "meta_desc_length", Type: FilterUint},
	"depth":              {Column: "depth", Type: FilterUint},
	"word_count":         {Column: "word_count", Type: FilterUint},
	"internal_links_out": {Column: "internal_links_out", Type: FilterUint},
	"external_links_out": {Column: "external_links_out", Type: FilterUint},
	"images_count":       {Column: "images_count", Type: FilterUint},
	"images_no_alt":      {Column: "images_no_alt", Type: FilterUint},
	"body_size":          {Column: "body_size", Type: FilterUint},
	"fetch_duration_ms":  {Column: "fetch_duration_ms", Type: FilterUint},
	"is_indexable":       {Column: "is_indexable", Type: FilterBool},
	"canonical_is_self":  {Column: "canonical_is_self", Type: FilterBool},
	"h1":                 {Column: "h1", Type: FilterArray},
	"h2":                 {Column: "h2", Type: FilterArray},
	"pagerank":           {Column: "pagerank", Type: FilterUint},
}

// LinkFilters defines the allowed filter columns for the links table.
var LinkFilters = map[string]FilterDef{
	"source_url":   {Column: "source_url", Type: FilterLike},
	"target_url":   {Column: "target_url", Type: FilterLike},
	"anchor_text":  {Column: "anchor_text", Type: FilterLike},
	"rel":          {Column: "rel", Type: FilterLike},
	"tag":          {Column: "tag", Type: FilterLike},
}

// ExternalCheckFilters defines the allowed filter columns for the external_link_checks table.
var ExternalCheckFilters = map[string]FilterDef{
	"url":          {Column: "url", Type: FilterLike},
	"status_code":  {Column: "status_code", Type: FilterUint},
	"error":        {Column: "error", Type: FilterLike},
	"content_type": {Column: "content_type", Type: FilterLike},
	"redirect_url": {Column: "redirect_url", Type: FilterLike},
}

// ExternalDomainCheckFilters defines the allowed filter columns for domain-level external checks.
var ExternalDomainCheckFilters = map[string]FilterDef{
	"domain": {Column: "domain", Type: FilterLike},
}

// BuildWhereClause generates a SQL WHERE clause fragment and arguments from parsed filters.
func BuildWhereClause(filters []ParsedFilter) (string, []interface{}, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}

	var clauses []string
	var args []interface{}

	for _, f := range filters {
		val := strings.TrimSpace(f.Value)
		if val == "" {
			continue
		}

		switch f.Def.Type {
		case FilterLike:
			clauses = append(clauses, fmt.Sprintf("%s ILIKE ?", f.Def.Column))
			args = append(args, "%"+val+"%")

		case FilterUint:
			// Support range syntax: "100-300" → col >= 100 AND col <= 300
			if lo, hi, ok := parseUintRange(val); ok {
				clauses = append(clauses, fmt.Sprintf("%s >= ? AND %s <= ?", f.Def.Column, f.Def.Column))
				args = append(args, lo, hi)
				continue
			}
			op, numStr := parseUintOp(val)
			n, err := strconv.ParseUint(numStr, 10, 64)
			if err != nil {
				return "", nil, fmt.Errorf("invalid numeric value for %s: %s", f.Def.Column, val)
			}
			clauses = append(clauses, fmt.Sprintf("%s %s ?", f.Def.Column, op))
			args = append(args, n)

		case FilterBool:
			lower := strings.ToLower(val)
			if lower != "true" && lower != "false" && lower != "1" && lower != "0" {
				return "", nil, fmt.Errorf("invalid bool value for %s: %s", f.Def.Column, val)
			}
			b := lower == "true" || lower == "1"
			clauses = append(clauses, fmt.Sprintf("%s = ?", f.Def.Column))
			args = append(args, b)

		case FilterArray:
			clauses = append(clauses, fmt.Sprintf("arrayExists(x -> x ILIKE ?, %s)", f.Def.Column))
			args = append(args, "%"+val+"%")
		}
	}

	if len(clauses) == 0 {
		return "", nil, nil
	}
	return strings.Join(clauses, " AND "), args, nil
}

// parseUintRange tries to parse "N-M" range syntax. Returns lo, hi, ok.
func parseUintRange(val string) (uint64, uint64, bool) {
	// Avoid matching operator prefixes like ">100"
	if len(val) > 0 && (val[0] == '>' || val[0] == '<' || val[0] == '=') {
		return 0, 0, false
	}
	parts := strings.SplitN(val, "-", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, false
	}
	lo, err1 := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 64)
	hi, err2 := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return lo, hi, true
}

// parseUintOp extracts a comparison operator prefix from a value string.
func parseUintOp(val string) (string, string) {
	if strings.HasPrefix(val, ">=") {
		return ">=", strings.TrimSpace(val[2:])
	}
	if strings.HasPrefix(val, "<=") {
		return "<=", strings.TrimSpace(val[2:])
	}
	if strings.HasPrefix(val, ">") {
		return ">", strings.TrimSpace(val[1:])
	}
	if strings.HasPrefix(val, "<") {
		return "<", strings.TrimSpace(val[1:])
	}
	return "=", val
}
