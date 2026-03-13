package schema

// ValidationLevel indicates severity of a validation issue.
type ValidationLevel string

const (
	LevelError   ValidationLevel = "error"
	LevelWarning ValidationLevel = "warning"
)

// FieldRule defines a required or recommended field for a schema type.
type FieldRule struct {
	Field   string          // JSON path: "headline", "offers.price"
	Level   ValidationLevel // "error" or "warning"
	Message string
}

// Rules maps Google-supported schema types to their validation rules.
// Error = required by Google, Warning = recommended.
var Rules = map[string][]FieldRule{
	"Article": {
		{Field: "headline", Level: LevelError, Message: "Missing required field 'headline'"},
		{Field: "image", Level: LevelError, Message: "Missing required field 'image'"},
		{Field: "datePublished", Level: LevelError, Message: "Missing required field 'datePublished'"},
		{Field: "author", Level: LevelError, Message: "Missing required field 'author'"},
		{Field: "dateModified", Level: LevelWarning, Message: "Recommended field 'dateModified' is missing"},
		{Field: "publisher", Level: LevelWarning, Message: "Recommended field 'publisher' is missing"},
	},
	"NewsArticle": {
		{Field: "headline", Level: LevelError, Message: "Missing required field 'headline'"},
		{Field: "image", Level: LevelError, Message: "Missing required field 'image'"},
		{Field: "datePublished", Level: LevelError, Message: "Missing required field 'datePublished'"},
		{Field: "author", Level: LevelError, Message: "Missing required field 'author'"},
		{Field: "dateModified", Level: LevelWarning, Message: "Recommended field 'dateModified' is missing"},
	},
	"BlogPosting": {
		{Field: "headline", Level: LevelError, Message: "Missing required field 'headline'"},
		{Field: "image", Level: LevelError, Message: "Missing required field 'image'"},
		{Field: "datePublished", Level: LevelError, Message: "Missing required field 'datePublished'"},
		{Field: "author", Level: LevelError, Message: "Missing required field 'author'"},
	},
	"Product": {
		{Field: "name", Level: LevelError, Message: "Missing required field 'name'"},
		{Field: "image", Level: LevelError, Message: "Missing required field 'image'"},
		{Field: "offers", Level: LevelWarning, Message: "Recommended field 'offers' is missing"},
		{Field: "offers.price", Level: LevelWarning, Message: "Recommended field 'offers.price' is missing"},
		{Field: "offers.priceCurrency", Level: LevelWarning, Message: "Recommended field 'offers.priceCurrency' is missing"},
		{Field: "offers.availability", Level: LevelWarning, Message: "Recommended field 'offers.availability' is missing"},
		{Field: "review", Level: LevelWarning, Message: "Recommended field 'review' is missing"},
		{Field: "aggregateRating", Level: LevelWarning, Message: "Recommended field 'aggregateRating' is missing"},
		{Field: "brand", Level: LevelWarning, Message: "Recommended field 'brand' is missing"},
		{Field: "description", Level: LevelWarning, Message: "Recommended field 'description' is missing"},
		{Field: "sku", Level: LevelWarning, Message: "Recommended field 'sku' is missing"},
	},
	"FAQPage": {
		{Field: "mainEntity", Level: LevelError, Message: "Missing required field 'mainEntity'"},
	},
	"HowTo": {
		{Field: "name", Level: LevelError, Message: "Missing required field 'name'"},
		{Field: "step", Level: LevelError, Message: "Missing required field 'step'"},
		{Field: "image", Level: LevelWarning, Message: "Recommended field 'image' is missing"},
		{Field: "totalTime", Level: LevelWarning, Message: "Recommended field 'totalTime' is missing"},
	},
	"LocalBusiness": {
		{Field: "name", Level: LevelError, Message: "Missing required field 'name'"},
		{Field: "address", Level: LevelError, Message: "Missing required field 'address'"},
		{Field: "image", Level: LevelWarning, Message: "Recommended field 'image' is missing"},
		{Field: "telephone", Level: LevelWarning, Message: "Recommended field 'telephone' is missing"},
		{Field: "openingHoursSpecification", Level: LevelWarning, Message: "Recommended field 'openingHoursSpecification' is missing"},
		{Field: "geo", Level: LevelWarning, Message: "Recommended field 'geo' is missing"},
		{Field: "url", Level: LevelWarning, Message: "Recommended field 'url' is missing"},
		{Field: "priceRange", Level: LevelWarning, Message: "Recommended field 'priceRange' is missing"},
	},
	"Recipe": {
		{Field: "name", Level: LevelError, Message: "Missing required field 'name'"},
		{Field: "image", Level: LevelError, Message: "Missing required field 'image'"},
		{Field: "recipeIngredient", Level: LevelWarning, Message: "Recommended field 'recipeIngredient' is missing"},
		{Field: "recipeInstructions", Level: LevelWarning, Message: "Recommended field 'recipeInstructions' is missing"},
		{Field: "cookTime", Level: LevelWarning, Message: "Recommended field 'cookTime' is missing"},
		{Field: "nutrition", Level: LevelWarning, Message: "Recommended field 'nutrition' is missing"},
		{Field: "aggregateRating", Level: LevelWarning, Message: "Recommended field 'aggregateRating' is missing"},
	},
	"Event": {
		{Field: "name", Level: LevelError, Message: "Missing required field 'name'"},
		{Field: "startDate", Level: LevelError, Message: "Missing required field 'startDate'"},
		{Field: "location", Level: LevelError, Message: "Missing required field 'location'"},
		{Field: "image", Level: LevelWarning, Message: "Recommended field 'image' is missing"},
		{Field: "description", Level: LevelWarning, Message: "Recommended field 'description' is missing"},
		{Field: "endDate", Level: LevelWarning, Message: "Recommended field 'endDate' is missing"},
		{Field: "offers", Level: LevelWarning, Message: "Recommended field 'offers' is missing"},
		{Field: "performer", Level: LevelWarning, Message: "Recommended field 'performer' is missing"},
		{Field: "organizer", Level: LevelWarning, Message: "Recommended field 'organizer' is missing"},
	},
	"BreadcrumbList": {
		{Field: "itemListElement", Level: LevelError, Message: "Missing required field 'itemListElement'"},
	},
	"VideoObject": {
		{Field: "name", Level: LevelError, Message: "Missing required field 'name'"},
		{Field: "description", Level: LevelError, Message: "Missing required field 'description'"},
		{Field: "thumbnailUrl", Level: LevelError, Message: "Missing required field 'thumbnailUrl'"},
		{Field: "uploadDate", Level: LevelError, Message: "Missing required field 'uploadDate'"},
		{Field: "contentUrl", Level: LevelWarning, Message: "Recommended field 'contentUrl' is missing"},
		{Field: "duration", Level: LevelWarning, Message: "Recommended field 'duration' is missing"},
		{Field: "embedUrl", Level: LevelWarning, Message: "Recommended field 'embedUrl' is missing"},
	},
	"Review": {
		{Field: "itemReviewed", Level: LevelError, Message: "Missing required field 'itemReviewed'"},
		{Field: "author", Level: LevelError, Message: "Missing required field 'author'"},
		{Field: "reviewRating", Level: LevelWarning, Message: "Recommended field 'reviewRating' is missing"},
		{Field: "datePublished", Level: LevelWarning, Message: "Recommended field 'datePublished' is missing"},
	},
	"Organization": {
		{Field: "name", Level: LevelError, Message: "Missing required field 'name'"},
		{Field: "url", Level: LevelWarning, Message: "Recommended field 'url' is missing"},
		{Field: "logo", Level: LevelWarning, Message: "Recommended field 'logo' is missing"},
		{Field: "contactPoint", Level: LevelWarning, Message: "Recommended field 'contactPoint' is missing"},
		{Field: "sameAs", Level: LevelWarning, Message: "Recommended field 'sameAs' is missing"},
	},
	"Person": {
		{Field: "name", Level: LevelError, Message: "Missing required field 'name'"},
		{Field: "url", Level: LevelWarning, Message: "Recommended field 'url' is missing"},
		{Field: "image", Level: LevelWarning, Message: "Recommended field 'image' is missing"},
	},
	"JobPosting": {
		{Field: "title", Level: LevelError, Message: "Missing required field 'title'"},
		{Field: "description", Level: LevelError, Message: "Missing required field 'description'"},
		{Field: "datePosted", Level: LevelError, Message: "Missing required field 'datePosted'"},
		{Field: "hiringOrganization", Level: LevelError, Message: "Missing required field 'hiringOrganization'"},
		{Field: "jobLocation", Level: LevelError, Message: "Missing required field 'jobLocation'"},
		{Field: "validThrough", Level: LevelWarning, Message: "Recommended field 'validThrough' is missing"},
		{Field: "employmentType", Level: LevelWarning, Message: "Recommended field 'employmentType' is missing"},
		{Field: "baseSalary", Level: LevelWarning, Message: "Recommended field 'baseSalary' is missing"},
	},
	"Course": {
		{Field: "name", Level: LevelError, Message: "Missing required field 'name'"},
		{Field: "description", Level: LevelError, Message: "Missing required field 'description'"},
		{Field: "provider", Level: LevelWarning, Message: "Recommended field 'provider' is missing"},
		{Field: "offers", Level: LevelWarning, Message: "Recommended field 'offers' is missing"},
	},
	"SoftwareApplication": {
		{Field: "name", Level: LevelError, Message: "Missing required field 'name'"},
		{Field: "offers", Level: LevelWarning, Message: "Recommended field 'offers' is missing"},
		{Field: "offers.price", Level: LevelWarning, Message: "Recommended field 'offers.price' is missing"},
		{Field: "aggregateRating", Level: LevelWarning, Message: "Recommended field 'aggregateRating' is missing"},
		{Field: "operatingSystem", Level: LevelWarning, Message: "Recommended field 'operatingSystem' is missing"},
		{Field: "applicationCategory", Level: LevelWarning, Message: "Recommended field 'applicationCategory' is missing"},
	},
}
