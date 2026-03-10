package storage

import (
	"testing"
)

// ---------------------------------------------------------------------------
// parseUintRange
// ---------------------------------------------------------------------------

func TestParseUintRange_Valid(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLo   uint64
		wantHi   uint64
		wantOK   bool
	}{
		{"simple range", "100-300", 100, 300, true},
		{"zero range", "0-0", 0, 0, true},
		{"with spaces", "100 - 300", 100, 300, true},
		{"large numbers", "1000000-9999999", 1000000, 9999999, true},
		{"same value", "42-42", 42, 42, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lo, hi, ok := parseUintRange(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("parseUintRange(%q): ok = %v, want %v", tc.input, ok, tc.wantOK)
			}
			if lo != tc.wantLo {
				t.Errorf("parseUintRange(%q): lo = %d, want %d", tc.input, lo, tc.wantLo)
			}
			if hi != tc.wantHi {
				t.Errorf("parseUintRange(%q): hi = %d, want %d", tc.input, hi, tc.wantHi)
			}
		})
	}
}

func TestParseUintRange_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"operator prefix >", ">100"},
		{"operator prefix <", "<100"},
		{"operator prefix =", "=100"},
		{"operator prefix >=", ">=100"},
		{"operator prefix <=", "<=100"},
		{"single number", "42"},
		{"non-numeric parts", "abc-def"},
		{"empty string", ""},
		{"missing left side", "-100"},
		{"missing right side", "100-"},
		{"non-numeric left", "abc-100"},
		{"non-numeric right", "100-abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ok := parseUintRange(tc.input)
			if ok {
				t.Errorf("parseUintRange(%q): expected ok=false, got true", tc.input)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseUintOp
// ---------------------------------------------------------------------------

func TestParseUintOp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOp  string
		wantVal string
	}{
		{"greater or equal", ">=50", ">=", "50"},
		{"less or equal", "<=10", "<=", "10"},
		{"greater than", ">5", ">", "5"},
		{"less than", "<3", "<", "3"},
		{"plain number defaults to =", "42", "=", "42"},
		{"explicit equals prefix", "=100", "=", "=100"},
		{"ge with spaces", ">= 50", ">=", "50"},
		{"le with spaces", "<= 10", "<=", "10"},
		{"gt with spaces", "> 5", ">", "5"},
		{"lt with spaces", "< 3", "<", "3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op, val := parseUintOp(tc.input)
			if op != tc.wantOp {
				t.Errorf("parseUintOp(%q): op = %q, want %q", tc.input, op, tc.wantOp)
			}
			if val != tc.wantVal {
				t.Errorf("parseUintOp(%q): val = %q, want %q", tc.input, val, tc.wantVal)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseSort
// ---------------------------------------------------------------------------

func TestParseSort(t *testing.T) {
	whitelist := map[string]string{
		"url":         "url",
		"status_code": "status_code",
		"depth":       "depth",
	}

	t.Run("empty key returns nil", func(t *testing.T) {
		got := ParseSort("", "ASC", whitelist)
		if got != nil {
			t.Errorf("expected nil for empty key, got %+v", got)
		}
	})

	t.Run("invalid key returns nil", func(t *testing.T) {
		got := ParseSort("nonexistent", "ASC", whitelist)
		if got != nil {
			t.Errorf("expected nil for invalid key, got %+v", got)
		}
	})

	t.Run("valid key with ASC", func(t *testing.T) {
		got := ParseSort("url", "ASC", whitelist)
		if got == nil {
			t.Fatal("expected non-nil SortParam")
		}
		if got.Column != "url" {
			t.Errorf("Column = %q, want %q", got.Column, "url")
		}
		if got.Order != "ASC" {
			t.Errorf("Order = %q, want %q", got.Order, "ASC")
		}
	})

	t.Run("valid key with DESC", func(t *testing.T) {
		got := ParseSort("status_code", "DESC", whitelist)
		if got == nil {
			t.Fatal("expected non-nil SortParam")
		}
		if got.Column != "status_code" {
			t.Errorf("Column = %q, want %q", got.Column, "status_code")
		}
		if got.Order != "DESC" {
			t.Errorf("Order = %q, want %q", got.Order, "DESC")
		}
	})

	t.Run("invalid order defaults to ASC", func(t *testing.T) {
		got := ParseSort("depth", "RANDOM", whitelist)
		if got == nil {
			t.Fatal("expected non-nil SortParam")
		}
		if got.Order != "ASC" {
			t.Errorf("Order = %q, want %q", got.Order, "ASC")
		}
	})

	t.Run("empty order defaults to ASC", func(t *testing.T) {
		got := ParseSort("depth", "", whitelist)
		if got == nil {
			t.Fatal("expected non-nil SortParam")
		}
		if got.Order != "ASC" {
			t.Errorf("Order = %q, want %q", got.Order, "ASC")
		}
	})

	t.Run("lowercase desc is uppercased", func(t *testing.T) {
		got := ParseSort("url", "desc", whitelist)
		if got == nil {
			t.Fatal("expected non-nil SortParam")
		}
		if got.Order != "DESC" {
			t.Errorf("Order = %q, want %q", got.Order, "DESC")
		}
	})

	t.Run("lowercase asc is uppercased", func(t *testing.T) {
		got := ParseSort("url", "asc", whitelist)
		if got == nil {
			t.Fatal("expected non-nil SortParam")
		}
		if got.Order != "ASC" {
			t.Errorf("Order = %q, want %q", got.Order, "ASC")
		}
	})
}

// ---------------------------------------------------------------------------
// BuildOrderByClause
// ---------------------------------------------------------------------------

func TestBuildOrderByClause(t *testing.T) {
	t.Run("nil sort uses default", func(t *testing.T) {
		got := BuildOrderByClause(nil, "created_at DESC")
		want := " ORDER BY created_at DESC"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("non-nil sort uses sort param", func(t *testing.T) {
		sp := &SortParam{Column: "url", Order: "ASC"}
		got := BuildOrderByClause(sp, "created_at DESC")
		want := " ORDER BY url ASC"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("sort DESC", func(t *testing.T) {
		sp := &SortParam{Column: "status_code", Order: "DESC"}
		got := BuildOrderByClause(sp, "id ASC")
		want := " ORDER BY status_code DESC"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// ---------------------------------------------------------------------------
// BuildWhereClause
// ---------------------------------------------------------------------------

func TestBuildWhereClause_Empty(t *testing.T) {
	t.Run("nil filters", func(t *testing.T) {
		clause, args, err := BuildWhereClause(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clause != "" {
			t.Errorf("expected empty clause, got %q", clause)
		}
		if args != nil {
			t.Errorf("expected nil args, got %v", args)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		clause, args, err := BuildWhereClause([]ParsedFilter{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clause != "" {
			t.Errorf("expected empty clause, got %q", clause)
		}
		if args != nil {
			t.Errorf("expected nil args, got %v", args)
		}
	})
}

func TestBuildWhereClause_EmptyValueSkipped(t *testing.T) {
	filters := []ParsedFilter{
		{Def: FilterDef{Column: "url", Type: FilterLike}, Value: ""},
		{Def: FilterDef{Column: "title", Type: FilterLike}, Value: "   "},
	}
	clause, args, err := BuildWhereClause(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clause != "" {
		t.Errorf("expected empty clause for blank values, got %q", clause)
	}
	if args != nil {
		t.Errorf("expected nil args, got %v", args)
	}
}

func TestBuildWhereClause_FilterLike(t *testing.T) {
	filters := []ParsedFilter{
		{Def: FilterDef{Column: "url", Type: FilterLike}, Value: "example"},
	}
	clause, args, err := BuildWhereClause(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantClause := "url ILIKE ?"
	if clause != wantClause {
		t.Errorf("clause = %q, want %q", clause, wantClause)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "%example%" {
		t.Errorf("arg[0] = %q, want %q", args[0], "%example%")
	}
}

func TestBuildWhereClause_FilterUint_PlainNumber(t *testing.T) {
	filters := []ParsedFilter{
		{Def: FilterDef{Column: "status_code", Type: FilterUint}, Value: "200"},
	}
	clause, args, err := BuildWhereClause(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantClause := "status_code = ?"
	if clause != wantClause {
		t.Errorf("clause = %q, want %q", clause, wantClause)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != uint64(200) {
		t.Errorf("arg[0] = %v, want %v", args[0], uint64(200))
	}
}

func TestBuildWhereClause_FilterUint_Operators(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantClause string
		wantArg    uint64
	}{
		{"greater or equal", ">=50", "depth >= ?", 50},
		{"less or equal", "<=10", "depth <= ?", 10},
		{"greater than", ">5", "depth > ?", 5},
		{"less than", "<3", "depth < ?", 3},
		{"plain equals", "100", "depth = ?", 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filters := []ParsedFilter{
				{Def: FilterDef{Column: "depth", Type: FilterUint}, Value: tc.value},
			}
			clause, args, err := BuildWhereClause(filters)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if clause != tc.wantClause {
				t.Errorf("clause = %q, want %q", clause, tc.wantClause)
			}
			if len(args) != 1 {
				t.Fatalf("expected 1 arg, got %d", len(args))
			}
			if args[0] != tc.wantArg {
				t.Errorf("arg[0] = %v, want %v", args[0], tc.wantArg)
			}
		})
	}
}

func TestBuildWhereClause_FilterUint_Range(t *testing.T) {
	filters := []ParsedFilter{
		{Def: FilterDef{Column: "status_code", Type: FilterUint}, Value: "200-299"},
	}
	clause, args, err := BuildWhereClause(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantClause := "status_code >= ? AND status_code <= ?"
	if clause != wantClause {
		t.Errorf("clause = %q, want %q", clause, wantClause)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != uint64(200) {
		t.Errorf("arg[0] = %v, want %v", args[0], uint64(200))
	}
	if args[1] != uint64(299) {
		t.Errorf("arg[1] = %v, want %v", args[1], uint64(299))
	}
}

func TestBuildWhereClause_FilterUint_EqualsPrefixSkipped(t *testing.T) {
	// "=100" → parseUintOp returns ("=", "=100"), ParseUint fails → silently skipped.
	filters := []ParsedFilter{
		{Def: FilterDef{Column: "depth", Type: FilterUint}, Value: "=100"},
	}
	clause, _, err := BuildWhereClause(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clause != "" {
		t.Errorf("expected empty clause for =100 prefix, got %q", clause)
	}
}

func TestBuildWhereClause_FilterUint_InvalidNumber(t *testing.T) {
	// Invalid numeric values are silently skipped (user input).
	filters := []ParsedFilter{
		{Def: FilterDef{Column: "status_code", Type: FilterUint}, Value: "abc"},
	}
	clause, _, err := BuildWhereClause(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clause != "" {
		t.Errorf("expected empty clause for invalid value, got %q", clause)
	}
}

func TestBuildWhereClause_FilterBool(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantArg bool
	}{
		{"true", "true", true},
		{"false", "false", false},
		{"1", "1", true},
		{"0", "0", false},
		{"TRUE uppercase", "TRUE", true},
		{"FALSE uppercase", "FALSE", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filters := []ParsedFilter{
				{Def: FilterDef{Column: "is_indexable", Type: FilterBool}, Value: tc.value},
			}
			clause, args, err := BuildWhereClause(filters)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			wantClause := "is_indexable = ?"
			if clause != wantClause {
				t.Errorf("clause = %q, want %q", clause, wantClause)
			}
			if len(args) != 1 {
				t.Fatalf("expected 1 arg, got %d", len(args))
			}
			if args[0] != tc.wantArg {
				t.Errorf("arg[0] = %v, want %v", args[0], tc.wantArg)
			}
		})
	}
}

func TestBuildWhereClause_FilterBool_Invalid(t *testing.T) {
	// Invalid bool values are silently skipped (user input).
	filters := []ParsedFilter{
		{Def: FilterDef{Column: "is_indexable", Type: FilterBool}, Value: "maybe"},
	}
	clause, _, err := BuildWhereClause(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clause != "" {
		t.Errorf("expected empty clause for invalid value, got %q", clause)
	}
}

func TestBuildWhereClause_FilterArray(t *testing.T) {
	filters := []ParsedFilter{
		{Def: FilterDef{Column: "h1", Type: FilterArray}, Value: "welcome"},
	}
	clause, args, err := BuildWhereClause(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantClause := "arrayExists(x -> x ILIKE ?, h1)"
	if clause != wantClause {
		t.Errorf("clause = %q, want %q", clause, wantClause)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "%welcome%" {
		t.Errorf("arg[0] = %q, want %q", args[0], "%welcome%")
	}
}

func TestBuildWhereClause_MultipleFilters(t *testing.T) {
	filters := []ParsedFilter{
		{Def: FilterDef{Column: "url", Type: FilterLike}, Value: "/blog"},
		{Def: FilterDef{Column: "status_code", Type: FilterUint}, Value: "200"},
		{Def: FilterDef{Column: "is_indexable", Type: FilterBool}, Value: "true"},
	}
	clause, args, err := BuildWhereClause(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantClause := "url ILIKE ? AND status_code = ? AND is_indexable = ?"
	if clause != wantClause {
		t.Errorf("clause = %q, want %q", clause, wantClause)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
	if args[0] != "%/blog%" {
		t.Errorf("arg[0] = %q, want %q", args[0], "%/blog%")
	}
	if args[1] != uint64(200) {
		t.Errorf("arg[1] = %v, want %v", args[1], uint64(200))
	}
	if args[2] != true {
		t.Errorf("arg[2] = %v, want %v", args[2], true)
	}
}

func TestBuildWhereClause_MixedEmptyAndNonEmpty(t *testing.T) {
	filters := []ParsedFilter{
		{Def: FilterDef{Column: "url", Type: FilterLike}, Value: ""},
		{Def: FilterDef{Column: "title", Type: FilterLike}, Value: "hello"},
		{Def: FilterDef{Column: "depth", Type: FilterUint}, Value: "  "},
	}
	clause, args, err := BuildWhereClause(filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantClause := "title ILIKE ?"
	if clause != wantClause {
		t.Errorf("clause = %q, want %q", clause, wantClause)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "%hello%" {
		t.Errorf("arg[0] = %q, want %q", args[0], "%hello%")
	}
}
