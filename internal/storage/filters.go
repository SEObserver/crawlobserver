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

// ProviderDataFilters defines the allowed filter columns for the provider_data table.
var ProviderDataFilters = map[string]FilterDef{
	"item_url":      {Column: "item_url", Type: FilterLike},
	"title":         {Column: "str_data['title']", Type: FilterLike},
	"language":      {Column: "str_data['language']", Type: FilterLike},
	"trust_flow":    {Column: "trust_flow", Type: FilterUint},
	"citation_flow": {Column: "citation_flow", Type: FilterUint},
	"ext_backlinks": {Column: "ext_backlinks", Type: FilterUint},
	"ref_domains":   {Column: "ref_domains", Type: FilterUint},
	"topic":         {Column: "str_data['ttf_topic_0']", Type: FilterLike},
}

// ProviderDataSortColumns maps query param names to DB column names for provider_data.
var ProviderDataSortColumns = map[string]string{
	"item_url":      "item_url",
	"trust_flow":    "trust_flow",
	"citation_flow": "citation_flow",
	"ext_backlinks": "ext_backlinks",
	"ref_domains":   "ref_domains",
	"domain_rank":   "domain_rank",
}

// LinkFilters defines the allowed filter columns for the links table.
var LinkFilters = map[string]FilterDef{
	"source_url":  {Column: "source_url", Type: FilterLike},
	"target_url":  {Column: "target_url", Type: FilterLike},
	"anchor_text": {Column: "anchor_text", Type: FilterLike},
	"rel":         {Column: "rel", Type: FilterLike},
	"tag":         {Column: "tag", Type: FilterLike},
}

// ExternalCheckFilters defines the allowed filter columns for the external_link_checks table.
var ExternalCheckFilters = map[string]FilterDef{
	"url":              {Column: "ec.url", Type: FilterLike},
	"status_code":      {Column: "ec.status_code", Type: FilterUint},
	"error":            {Column: "ec.error", Type: FilterLike},
	"content_type":     {Column: "ec.content_type", Type: FilterLike},
	"redirect_url":     {Column: "ec.redirect_url", Type: FilterLike},
	"response_time_ms": {Column: "ec.response_time_ms", Type: FilterUint},
	"source_url":       {Column: "l.source_url", Type: FilterLike},
	"source_depth":     {Column: "p.depth", Type: FilterUint},
}

// ExternalCheckSortColumns maps query param names to DB column names for external_link_checks.
var ExternalCheckSortColumns = map[string]string{
	"url":              "ec.url",
	"status_code":      "ec.status_code",
	"error":            "ec.error",
	"content_type":     "ec.content_type",
	"response_time_ms": "ec.response_time_ms",
	"source_url":       "l.source_url",
	"source_pagerank":  "p.pagerank",
	"source_depth":     "p.depth",
}

// ExternalDomainCheckFilters defines the allowed filter columns for domain-level external checks.
// Note: "unreachable" is handled as a HAVING clause in the query since it's an aggregate alias.
var ExternalDomainCheckFilters = map[string]FilterDef{
	"domain": {Column: "domain", Type: FilterLike},
}

// ExternalDomainCheckHavingFilters defines aggregate-level filters applied via HAVING.
var ExternalDomainCheckHavingFilters = map[string]FilterDef{
	"unreachable": {Column: "unreachable", Type: FilterUint},
	"ns_dead":     {Column: "ns_dead", Type: FilterUint},
}

// ExternalDomainCheckSortColumns maps query param names to DB column names for domain-level external checks.
var ExternalDomainCheckSortColumns = map[string]string{
	"domain":          "domain",
	"total_urls":      "total_urls",
	"ok":              "ok",
	"redirects":       "redirects",
	"client_errors":   "client_errors",
	"server_errors":   "server_errors",
	"unreachable":     "unreachable",
	"ns_dead":         "ns_dead",
	"avg_response_ms": "avg_response_ms",
}

// PageResourceCheckFilters defines the allowed filter columns for page_resource_checks.
var PageResourceCheckFilters = map[string]FilterDef{
	"url":           {Column: "url", Type: FilterLike},
	"resource_type": {Column: "resource_type", Type: FilterLike},
	"is_internal":   {Column: "is_internal", Type: FilterBool},
	"status_code":   {Column: "status_code", Type: FilterUint},
	"content_type":  {Column: "content_type", Type: FilterLike},
	"error":         {Column: "error", Type: FilterLike},
}

// RedirectFilters defines the allowed filter columns for the redirect pages view.
var RedirectFilters = map[string]FilterDef{
	"url":         {Column: "p.url", Type: FilterLike},
	"status_code": {Column: "p.status_code", Type: FilterUint},
	"final_url":   {Column: "p.final_url", Type: FilterLike},
}

// RedirectSortColumns maps query param names to DB column names for redirect pages.
var RedirectSortColumns = map[string]string{
	"url":                    "p.url",
	"status_code":            "p.status_code",
	"final_url":              "p.final_url",
	"inbound_internal_links": "inbound_internal_links",
}

// SortParam holds a validated sort column and direction.
type SortParam struct {
	Column string // DB column name (from whitelist)
	Order  string // "ASC" or "DESC"
}

// BacklinkFilters defines the allowed filter columns for provider_backlinks.
var BacklinkFilters = map[string]FilterDef{
	"source_url":  {Column: "source_url", Type: FilterLike},
	"target_url":  {Column: "target_url", Type: FilterLike},
	"anchor_text": {Column: "anchor_text", Type: FilterLike},
	"trust_flow":  {Column: "domain_rank", Type: FilterUint},
	"citation_flow": {Column: "page_rank", Type: FilterUint},
	"nofollow":    {Column: "nofollow", Type: FilterBool},
	"first_seen":  {Column: "first_seen", Type: FilterLike},
	"last_seen":   {Column: "last_seen", Type: FilterLike},
}

// BacklinkSortColumns maps query param names to DB column names for provider_backlinks.
var BacklinkSortColumns = map[string]string{
	"source_url":    "source_url",
	"target_url":    "target_url",
	"anchor_text":   "anchor_text",
	"trust_flow":    "domain_rank",
	"citation_flow": "page_rank",
	"nofollow":      "nofollow",
	"first_seen":    "first_seen",
	"last_seen":     "last_seen",
}

// PageSortColumns maps query param names to DB column names for pages.
var PageSortColumns = map[string]string{
	"url":                "url",
	"status_code":        "status_code",
	"title":              "title",
	"title_length":       "title_length",
	"word_count":         "word_count",
	"internal_links_out": "internal_links_out",
	"external_links_out": "external_links_out",
	"body_size":          "body_size",
	"fetch_duration_ms":  "fetch_duration_ms",
	"depth":              "depth",
	"pagerank":           "pagerank",
	"content_type":       "content_type",
	"meta_description":   "meta_description",
	"meta_desc_length":   "meta_desc_length",
	"meta_keywords":      "meta_keywords",
	"canonical":          "canonical",
	"is_indexable":       "is_indexable",
	"index_reason":       "index_reason",
	"meta_robots":        "meta_robots",
	"canonical_is_self":  "canonical_is_self",
	"images_count":       "images_count",
	"images_no_alt":      "images_no_alt",
	"content_encoding":   "content_encoding",
	"lang":               "lang",
	"og_title":           "og_title",
	"crawled_at":         "crawled_at",
}

// LinkSortColumns maps query param names to DB column names for links.
var LinkSortColumns = map[string]string{
	"source_url":  "source_url",
	"target_url":  "target_url",
	"anchor_text": "anchor_text",
	"rel":         "rel",
	"tag":         "tag",
	"crawled_at":  "crawled_at",
}

// ParseSort validates sort/order params against a whitelist and returns a SortParam or nil.
func ParseSort(sortKey, orderStr string, whitelist map[string]string) *SortParam {
	if sortKey == "" {
		return nil
	}
	col, ok := whitelist[sortKey]
	if !ok {
		return nil
	}
	order := strings.ToUpper(orderStr)
	if order != "ASC" && order != "DESC" {
		order = "ASC"
	}
	return &SortParam{Column: col, Order: order}
}

// BuildOrderByClause returns an ORDER BY clause using the sort param or the default.
func BuildOrderByClause(sort *SortParam, defaultOrderBy string) string {
	if sort == nil {
		return " ORDER BY " + defaultOrderBy
	}
	return fmt.Sprintf(" ORDER BY %s %s", sort.Column, sort.Order)
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
				// Skip invalid numeric values silently (user input)
				continue
			}
			clauses = append(clauses, fmt.Sprintf("%s %s ?", f.Def.Column, op))
			args = append(args, n)

		case FilterBool:
			lower := strings.ToLower(val)
			if lower != "true" && lower != "false" && lower != "1" && lower != "0" {
				// Skip invalid bool values silently (user input)
				continue
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
