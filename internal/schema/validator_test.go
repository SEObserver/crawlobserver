package schema

import (
	"testing"
	"time"
)

func TestValidateBlock_Article(t *testing.T) {
	jsonLD := `{"@type":"Article","headline":"Test","image":"img.jpg","datePublished":"2024-01-01","author":{"@type":"Person","name":"Bob"}}`
	results := ValidateBlock(jsonLD)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].SchemaType != "Article" {
		t.Errorf("expected Article, got %s", results[0].SchemaType)
	}
	if !results[0].IsValid {
		t.Error("expected valid Article")
	}
	if len(results[0].Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(results[0].Errors))
	}
}

func TestValidateBlock_ArticleMissingFields(t *testing.T) {
	jsonLD := `{"@type":"Article"}`
	results := ValidateBlock(jsonLD)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].IsValid {
		t.Error("expected invalid Article")
	}
	if len(results[0].Errors) != 4 {
		t.Errorf("expected 4 errors (headline, image, datePublished, author), got %d", len(results[0].Errors))
	}
}

func TestValidateBlock_Product(t *testing.T) {
	jsonLD := `{"@type":"Product","name":"Widget","image":"img.jpg","offers":{"@type":"Offer","price":"9.99","priceCurrency":"USD","availability":"InStock"}}`
	results := ValidateBlock(jsonLD)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsValid {
		t.Error("expected valid Product")
	}
}

func TestValidateBlock_ProductMissingName(t *testing.T) {
	jsonLD := `{"@type":"Product","image":"img.jpg"}`
	results := ValidateBlock(jsonLD)
	if results[0].IsValid {
		t.Error("Product without name should be invalid")
	}
}

func TestValidateBlock_FAQPage(t *testing.T) {
	jsonLD := `{"@type":"FAQPage","mainEntity":[{"@type":"Question","name":"Q?","acceptedAnswer":{"@type":"Answer","text":"A"}}]}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid FAQPage")
	}
}

func TestValidateBlock_FAQPageMissing(t *testing.T) {
	jsonLD := `{"@type":"FAQPage"}`
	results := ValidateBlock(jsonLD)
	if results[0].IsValid {
		t.Error("FAQPage without mainEntity should be invalid")
	}
}

func TestValidateBlock_HowTo(t *testing.T) {
	jsonLD := `{"@type":"HowTo","name":"Fix bike","step":[{"@type":"HowToStep","text":"Remove wheel"}]}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid HowTo")
	}
}

func TestValidateBlock_LocalBusiness(t *testing.T) {
	jsonLD := `{"@type":"LocalBusiness","name":"Cafe","address":{"@type":"PostalAddress","streetAddress":"123 Main St"}}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid LocalBusiness")
	}
}

func TestValidateBlock_Recipe(t *testing.T) {
	jsonLD := `{"@type":"Recipe","name":"Cake","image":"cake.jpg"}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid Recipe")
	}
}

func TestValidateBlock_Event(t *testing.T) {
	jsonLD := `{"@type":"Event","name":"Concert","startDate":"2024-06-15","location":{"@type":"Place","name":"Arena"}}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid Event")
	}
}

func TestValidateBlock_EventMissing(t *testing.T) {
	jsonLD := `{"@type":"Event"}`
	results := ValidateBlock(jsonLD)
	if results[0].IsValid {
		t.Error("Event without required fields should be invalid")
	}
	if len(results[0].Errors) != 3 {
		t.Errorf("expected 3 errors, got %d", len(results[0].Errors))
	}
}

func TestValidateBlock_BreadcrumbList(t *testing.T) {
	jsonLD := `{"@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Home"}]}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid BreadcrumbList")
	}
}

func TestValidateBlock_VideoObject(t *testing.T) {
	jsonLD := `{"@type":"VideoObject","name":"Video","description":"A video","thumbnailUrl":"thumb.jpg","uploadDate":"2024-01-01"}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid VideoObject")
	}
}

func TestValidateBlock_Review(t *testing.T) {
	jsonLD := `{"@type":"Review","itemReviewed":{"@type":"Product","name":"X"},"author":{"@type":"Person","name":"Bob"}}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid Review")
	}
}

func TestValidateBlock_Organization(t *testing.T) {
	jsonLD := `{"@type":"Organization","name":"Acme"}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid Organization")
	}
}

func TestValidateBlock_Person(t *testing.T) {
	jsonLD := `{"@type":"Person","name":"Alice"}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid Person")
	}
}

func TestValidateBlock_JobPosting(t *testing.T) {
	jsonLD := `{"@type":"JobPosting","title":"Dev","description":"Build stuff","datePosted":"2024-01-01","hiringOrganization":{"@type":"Organization","name":"X"},"jobLocation":{"@type":"Place","address":"NYC"}}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid JobPosting")
	}
}

func TestValidateBlock_Course(t *testing.T) {
	jsonLD := `{"@type":"Course","name":"Go 101","description":"Learn Go"}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid Course")
	}
}

func TestValidateBlock_SoftwareApplication(t *testing.T) {
	jsonLD := `{"@type":"SoftwareApplication","name":"MyApp"}`
	results := ValidateBlock(jsonLD)
	if !results[0].IsValid {
		t.Error("expected valid SoftwareApplication")
	}
}

func TestValidateBlock_Graph(t *testing.T) {
	jsonLD := `{"@graph":[{"@type":"Organization","name":"Acme"},{"@type":"Person","name":"Bob"}]}`
	results := ValidateBlock(jsonLD)
	if len(results) != 2 {
		t.Fatalf("expected 2 results from @graph, got %d", len(results))
	}
	for _, r := range results {
		if !r.IsValid {
			t.Errorf("expected valid %s", r.SchemaType)
		}
	}
}

func TestValidateBlock_ArrayOfObjects(t *testing.T) {
	jsonLD := `[{"@type":"Organization","name":"A"},{"@type":"Person","name":"B"}]`
	results := ValidateBlock(jsonLD)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestValidateBlock_UnknownType(t *testing.T) {
	jsonLD := `{"@type":"WebPage","name":"Test"}`
	results := ValidateBlock(jsonLD)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsValid {
		t.Error("unknown types should be considered valid (no rules)")
	}
}

func TestValidateBlock_EmptyInput(t *testing.T) {
	results := ValidateBlock("")
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}

func TestValidateBlock_InvalidJSON(t *testing.T) {
	results := ValidateBlock("{invalid json}")
	if len(results) != 0 {
		t.Errorf("expected 0 results for invalid JSON, got %d", len(results))
	}
}

func TestValidateBlock_NestedField(t *testing.T) {
	// Product with offers but no price
	jsonLD := `{"@type":"Product","name":"X","image":"x.jpg","offers":{"@type":"Offer"}}`
	results := ValidateBlock(jsonLD)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// offers exists but offers.price does not — should have warning
	hasWarning := false
	for _, w := range results[0].Warnings {
		if w.Message == "Recommended field 'offers.price' is missing" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("expected warning for missing offers.price")
	}
}

func TestValidateBlock_MultipleTypes(t *testing.T) {
	jsonLD := `{"@type":["Restaurant","LocalBusiness"],"name":"Cafe","address":"123 Main"}`
	results := ValidateBlock(jsonLD)
	// Should validate both types; Restaurant is unknown (no rules), LocalBusiness has rules
	found := false
	for _, r := range results {
		if r.SchemaType == "LocalBusiness" {
			found = true
		}
	}
	if !found {
		t.Error("expected LocalBusiness in results")
	}
}

func TestValidateAllBlocks(t *testing.T) {
	blocks := []string{
		`{"@type":"Article","headline":"Hi","image":"i.jpg","datePublished":"2024","author":"Bob"}`,
		`{"@type":"Product"}`,
	}
	items := ValidateAllBlocks(blocks, "session-1", "https://example.com", time.Now(), "static")
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// Article should be valid
	if !items[0].IsValid {
		t.Error("expected Article to be valid")
	}
	// Product missing name should be invalid
	if items[1].IsValid {
		t.Error("expected Product to be invalid")
	}
	if items[1].Source != "static" {
		t.Errorf("expected source 'static', got '%s'", items[1].Source)
	}
}

func TestCountSummary(t *testing.T) {
	items := []StructuredDataItem{
		{IsValid: true, Errors: nil, Warnings: []string{"w1"}},
		{IsValid: false, Errors: []string{"e1"}, Warnings: nil},
		{IsValid: true, Errors: nil, Warnings: nil},
	}
	v, e, w := CountSummary(items)
	if v != 2 {
		t.Errorf("expected 2 valid, got %d", v)
	}
	if e != 1 {
		t.Errorf("expected 1 with errors, got %d", e)
	}
	if w != 1 {
		t.Errorf("expected 1 with warnings, got %d", w)
	}
}
