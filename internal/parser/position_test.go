package parser

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// linksFrom parses an HTML fragment and returns the extracted links with their
// position filled in.
func linksFrom(t *testing.T, markup string) []Link {
	t.Helper()
	base, err := url.Parse("https://example.com/page")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	return extractLinks(docFromHTML(markup), base, Options{LinkPosition: true})
}

// linkByAnchor finds an extracted link by its anchor text.
func linkByAnchor(t *testing.T, links []Link, anchor string) Link {
	t.Helper()
	for _, l := range links {
		if l.AnchorText == anchor {
			return l
		}
	}
	t.Fatalf("no link with anchor %q in %+v", anchor, links)
	return Link{}
}

const landmarkHTML = `<!DOCTYPE html>
<html>
<body>
	<header><a href="/logo">Logo</a></header>
	<nav><ul><li><a href="/one">One</a></li><li><a href="/two">Two</a></li></ul></nav>
	<main><p><a href="/in-main">In main</a></p></main>
	<article><a href="/in-article">In article</a></article>
	<aside><a href="/in-aside">In aside</a></aside>
	<footer><a href="/in-footer">In footer</a></footer>
	<div><a href="/nowhere">Nowhere</a></div>
</body>
</html>`

func TestExtractLinksLandmark(t *testing.T) {
	links := linksFrom(t, landmarkHTML)

	want := map[string]string{
		"Logo":       "header",
		"One":        "nav",
		"In main":    "main",
		"In article": "article",
		"In aside":   "aside",
		"In footer":  "footer",
		"Nowhere":    "",
	}
	for anchor, landmark := range want {
		if got := linkByAnchor(t, links, anchor).Landmark; got != landmark {
			t.Errorf("Landmark for %q = %q, want %q", anchor, got, landmark)
		}
	}
}

func TestExtractLinksLandmarkNested(t *testing.T) {
	// The nearest sectioning ancestor wins, not the outermost.
	links := linksFrom(t, `<html><body><main><nav><a href="/x">Deep</a></nav></main></body></html>`)

	if got := linkByAnchor(t, links, "Deep").Landmark; got != "nav" {
		t.Errorf("Landmark = %q, want %q", got, "nav")
	}
}

func TestExtractLinksLandmarkFromARIARole(t *testing.T) {
	// Pages that use no HTML5 sectioning element fall back to ARIA roles,
	// reported with the same vocabulary as the elements they stand for.
	links := linksFrom(t, `<html><body>
		<div role="navigation"><a href="/a">Nav role</a></div>
		<div role="banner"><a href="/b">Banner role</a></div>
		<div role="contentinfo"><a href="/c">Contentinfo role</a></div>
		<div role="complementary"><a href="/d">Complementary role</a></div>
		<div role="main"><a href="/e">Main role</a></div>
		<div role="button"><a href="/f">Not a landmark</a></div>
	</body></html>`)

	want := map[string]string{
		"Nav role":           "nav",
		"Banner role":        "header",
		"Contentinfo role":   "footer",
		"Complementary role": "aside",
		"Main role":          "main",
		"Not a landmark":     "",
	}
	for anchor, landmark := range want {
		if got := linkByAnchor(t, links, anchor).Landmark; got != landmark {
			t.Errorf("Landmark for %q = %q, want %q", anchor, got, landmark)
		}
	}
}

func TestExtractLinksLandmarkRoleTokenList(t *testing.T) {
	// A role attribute holds a list of fallback roles; the first landmark wins.
	links := linksFrom(t, `<html><body><div role="doc-index navigation"><a href="/x">Listed</a></div></body></html>`)

	if got := linkByAnchor(t, links, "Listed").Landmark; got != "nav" {
		t.Errorf("Landmark = %q, want %q", got, "nav")
	}
}

func TestExtractLinksLandmarkElementBeatsRole(t *testing.T) {
	// Sectioning elements are searched through the whole ancestor chain before
	// any role is considered, so a role nested inside an element does not win.
	links := linksFrom(t, `<html><body><footer><div role="navigation"><a href="/x">Both</a></div></footer></body></html>`)

	if got := linkByAnchor(t, links, "Both").Landmark; got != "footer" {
		t.Errorf("Landmark = %q, want %q", got, "footer")
	}
}

func TestExtractLinksXPath(t *testing.T) {
	links := linksFrom(t, landmarkHTML)

	want := map[string]string{
		"Logo":      "/html/body/header/a",
		"One":       "/html/body/nav/ul/li[1]/a",
		"Two":       "/html/body/nav/ul/li[2]/a",
		"In main":   "/html/body/main/p/a",
		"In footer": "/html/body/footer/a",
	}
	for anchor, xpath := range want {
		if got := linkByAnchor(t, links, anchor).XPath; got != xpath {
			t.Errorf("XPath for %q = %q, want %q", anchor, got, xpath)
		}
	}
}

func TestExtractLinksXPathSiblingIndex(t *testing.T) {
	// The predicate counts same-named siblings only, and is left out when the
	// element is the only one of its name among its siblings.
	links := linksFrom(t, `<html><body><div><p>x</p><span><a href="/a">First span</a></span><span><a href="/b">Second span</a></span></div></body></html>`)

	if got := linkByAnchor(t, links, "First span").XPath; got != "/html/body/div/span[1]/a" {
		t.Errorf("XPath = %q, want %q", got, "/html/body/div/span[1]/a")
	}
	if got := linkByAnchor(t, links, "Second span").XPath; got != "/html/body/div/span[2]/a" {
		t.Errorf("XPath = %q, want %q", got, "/html/body/div/span[2]/a")
	}
}

func TestExtractLinksXPathSiblingLinks(t *testing.T) {
	// Two links under the same parent must not share a path.
	links := linksFrom(t, `<html><body><p><a href="/a">A</a> <a href="/b">B</a></p></body></html>`)

	if got := linkByAnchor(t, links, "A").XPath; got != "/html/body/p/a[1]" {
		t.Errorf("XPath = %q, want %q", got, "/html/body/p/a[1]")
	}
	if got := linkByAnchor(t, links, "B").XPath; got != "/html/body/p/a[2]" {
		t.Errorf("XPath = %q, want %q", got, "/html/body/p/a[2]")
	}
}

func TestExtractLinksDepth(t *testing.T) {
	links := linksFrom(t, landmarkHTML)

	want := map[string]uint16{
		"Logo":    3, // html, body, header
		"One":     5, // html, body, nav, ul, li
		"In main": 4, // html, body, main, p
	}
	for anchor, depth := range want {
		if got := linkByAnchor(t, links, anchor).Depth; got != depth {
			t.Errorf("Depth for %q = %d, want %d", anchor, got, depth)
		}
	}
}

func TestExtractLinksDocumentIndex(t *testing.T) {
	// The index ranks the links that are actually extracted, in document order:
	// skipped hrefs do not leave a hole in the numbering.
	links := linksFrom(t, `<html><body>
		<a href="/first">First</a>
		<a href="mailto:x@example.com">Skipped</a>
		<a href="#anchor">Skipped too</a>
		<a href="/second">Second</a>
		<a href="/third">Third</a>
	</body></html>`)

	if len(links) != 3 {
		t.Fatalf("len(links) = %d, want 3: %+v", len(links), links)
	}
	want := map[string]uint32{"First": 0, "Second": 1, "Third": 2}
	for anchor, index := range want {
		if got := linkByAnchor(t, links, anchor).DocumentIndex; got != index {
			t.Errorf("DocumentIndex for %q = %d, want %d", anchor, got, index)
		}
	}
}

func TestExtractLinksPositionLimitRetainsAllLinks(t *testing.T) {
	base, err := url.Parse("https://example.com/page")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	links := extractLinks(docFromHTML(`<html><body>
		<a href="/first">First</a>
		<a href="mailto:x@example.com">Skipped</a>
		<a href="#anchor">Skipped too</a>
		<a href="/second">Second</a>
		<a href="/third">Third</a>
	</body></html>`), base, Options{LinkPosition: true, MaxLinkPositions: 2})

	if len(links) != 3 {
		t.Fatalf("len(links) = %d, want 3: %+v", len(links), links)
	}
	if got := linkByAnchor(t, links, "First").DocumentIndex; got != 0 {
		t.Errorf("DocumentIndex for First = %d, want 0", got)
	}
	if got := linkByAnchor(t, links, "Second").DocumentIndex; got != 1 {
		t.Errorf("DocumentIndex for Second = %d, want 1", got)
	}
	third := linkByAnchor(t, links, "Third")
	if third.TargetURL != "https://example.com/third" {
		t.Errorf("TargetURL for uncategorized link = %q, want %q", third.TargetURL, "https://example.com/third")
	}
	if third.Landmark != "" || third.XPath != "" || third.Depth != 0 || third.DocumentIndex != 0 || third.BlockSignature != 0 {
		t.Errorf("position recorded beyond limit: %+v", third)
	}
}

func TestExtractLinksPositionLimitKeepsXPathSiblingCounts(t *testing.T) {
	base, err := url.Parse("https://example.com/page")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	links := extractLinks(docFromHTML(`<html><body><p>
		<a href="/before">Before</a>
		<a href="/target">Target</a>
		<a href="/after">After</a>
	</p></body></html>`), base, Options{LinkPosition: true, MaxLinkPositions: 2})

	if got := linkByAnchor(t, links, "Before").XPath; got != "/html/body/p/a[1]" {
		t.Errorf("XPath for Before = %q, want %q", got, "/html/body/p/a[1]")
	}
	if got := linkByAnchor(t, links, "Target").XPath; got != "/html/body/p/a[2]" {
		t.Errorf("XPath for Target = %q, want %q", got, "/html/body/p/a[2]")
	}
	if got := linkByAnchor(t, links, "After").XPath; got != "" {
		t.Errorf("XPath for link beyond limit = %q, want empty", got)
	}
}

func TestExtractLinksNonPositivePositionLimitUsesDefault(t *testing.T) {
	markup := `<html><body>` + strings.Repeat(`<a href="/link">Link</a>`, DefaultMaxLinkPositions+1) + `</body></html>`
	base, err := url.Parse("https://example.com/page")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	for _, limit := range []int{0, -1} {
		t.Run("limit="+strconv.Itoa(limit), func(t *testing.T) {
			links := extractLinks(docFromHTML(markup), base, Options{LinkPosition: true, MaxLinkPositions: limit})
			if len(links) != DefaultMaxLinkPositions+1 {
				t.Fatalf("len(links) = %d, want %d", len(links), DefaultMaxLinkPositions+1)
			}
			if links[DefaultMaxLinkPositions-1].XPath == "" {
				t.Errorf("link at default limit did not receive position metadata")
			}
			last := links[DefaultMaxLinkPositions]
			if last.XPath != "" || last.Landmark != "" || last.Depth != 0 || last.DocumentIndex != 0 || last.BlockSignature != 0 {
				t.Errorf("link beyond default limit has position metadata: %+v", last)
			}
		})
	}
}

// Two pages of the same site: the menu is identical markup, the article is not,
// and the body carries a per-page class the way many CMSes emit one.
const signaturePageOne = `<html><body class="page-id-11 single">
	<nav class="menu"><ul><li><a href="/one">One</a></li><li><a href="/two">Two</a></li></ul></nav>
	<main><p><a href="/story-a">Story A</a></p></main>
</body></html>`

const signaturePageTwo = `<html><body class="page-id-97 archive">
	<nav class="menu"><ul><li><a href="/one">One</a></li><li><a href="/two">Two</a></li></ul></nav>
	<main><p><a href="/story-b">Story B</a></p></main>
</body></html>`

func TestBlockSignatureRepeatsAcrossPages(t *testing.T) {
	one := linksFrom(t, signaturePageOne)
	two := linksFrom(t, signaturePageTwo)

	first := linkByAnchor(t, one, "One").BlockSignature
	second := linkByAnchor(t, two, "One").BlockSignature
	if first != second {
		t.Errorf("menu signature differs across pages: %d vs %d", first, second)
	}
	if first == 0 {
		t.Error("menu signature is zero, want a computed value")
	}
}

func TestBlockSignatureGroupsSiblingsOfSameBlock(t *testing.T) {
	// Every item of one menu belongs to the same block; telling the items apart
	// is what the XPath is for.
	links := linksFrom(t, signaturePageOne)

	if a, b := linkByAnchor(t, links, "One").BlockSignature, linkByAnchor(t, links, "Two").BlockSignature; a != b {
		t.Errorf("menu items have different signatures: %d vs %d", a, b)
	}
}

func TestBlockSignatureSeparatesDifferentBlocks(t *testing.T) {
	links := linksFrom(t, signaturePageOne)

	menu := linkByAnchor(t, links, "One").BlockSignature
	body := linkByAnchor(t, links, "Story A").BlockSignature
	if menu == body {
		t.Errorf("menu and article share signature %d, want different blocks", menu)
	}
}

func TestBlockSignatureSeparatesSiblingModules(t *testing.T) {
	// Two modules built from the same elements but named differently are two
	// blocks, not one.
	links := linksFrom(t, `<html><body><main>
		<div class="related-products"><a href="/a">In related</a></div>
		<div class="recent-posts"><a href="/b">In recent</a></div>
	</main></body></html>`)

	if a, b := linkByAnchor(t, links, "In related").BlockSignature, linkByAnchor(t, links, "In recent").BlockSignature; a == b {
		t.Errorf("both modules signed %d, want different blocks", a)
	}
}

func TestBlockSignatureUsesWholeChain(t *testing.T) {
	// Two blocks whose innermost markup is identical are still two blocks when
	// what wraps them differs.
	links := linksFrom(t, `<html><body>
		<nav class="primary"><ul><li><a href="/a">In primary</a></li></ul></nav>
		<nav class="secondary"><ul><li><a href="/b">In secondary</a></li></ul></nav>
	</body></html>`)

	if a, b := linkByAnchor(t, links, "In primary").BlockSignature, linkByAnchor(t, links, "In secondary").BlockSignature; a == b {
		t.Errorf("both menus signed %d, want different blocks", a)
	}
}

func TestBlockSignatureIgnoresClassOrder(t *testing.T) {
	one := linksFrom(t, `<html><body><div class="promo wide"><a href="/a">X</a></div></body></html>`)
	two := linksFrom(t, `<html><body><div class="wide promo"><a href="/a">X</a></div></body></html>`)

	if a, b := one[0].BlockSignature, two[0].BlockSignature; a != b {
		t.Errorf("class order changed the signature: %d vs %d", a, b)
	}
}

func TestExtractLinksPositionDisabled(t *testing.T) {
	base, err := url.Parse("https://example.com/page")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	links := extractLinks(docFromHTML(landmarkHTML), base, Options{LinkPosition: false})

	if len(links) == 0 {
		t.Fatal("expected links to be extracted with position turned off")
	}
	for _, l := range links {
		if l.Landmark != "" || l.XPath != "" || l.Depth != 0 || l.DocumentIndex != 0 || l.BlockSignature != 0 {
			t.Errorf("position recorded with LinkPosition off: %+v", l)
		}
	}
	// The rest of the link must still be extracted.
	if got := linkByAnchor(t, links, "One").TargetURL; got != "https://example.com/one" {
		t.Errorf("TargetURL = %q, want %q", got, "https://example.com/one")
	}
}

func TestParseFillsLinkPositionByDefault(t *testing.T) {
	data, err := Parse([]byte(landmarkHTML), "https://example.com/page")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	link := linkByAnchor(t, data.Links, "One")
	if link.XPath != "/html/body/nav/ul/li[1]/a" {
		t.Errorf("XPath = %q, want %q", link.XPath, "/html/body/nav/ul/li[1]/a")
	}
	if link.Landmark != "nav" {
		t.Errorf("Landmark = %q, want %q", link.Landmark, "nav")
	}
}

func TestExtractLinksDepthDirectlyInBody(t *testing.T) {
	// The documented reference point: html and body are the only ancestors.
	links := linksFrom(t, `<html><body><a href="/x">Bare</a></body></html>`)

	if got := linkByAnchor(t, links, "Bare").Depth; got != 2 {
		t.Errorf("Depth = %d, want 2", got)
	}
}

func TestBlockSignatureIgnoresBodyAndHTMLAttributes(t *testing.T) {
	// Links sitting directly in the body have an empty ancestor chain, so the
	// per-page attributes a CMS stamps on <body> and <html> cannot reach it.
	one := linksFrom(t, `<html lang="en" class="no-js"><body class="page-id-11"><a href="/x">Bare</a></body></html>`)
	two := linksFrom(t, `<html lang="fr" class="js"><body class="page-id-97 archive"><a href="/x">Bare</a></body></html>`)

	if a, b := one[0].BlockSignature, two[0].BlockSignature; a != b {
		t.Errorf("body and html attributes changed the signature: %d vs %d", a, b)
	}
}

func TestBlockSignatureSplitsOnCurrentPageMarkup(t *testing.T) {
	// A documented limit: markup that changes with the page being viewed, such
	// as the "current" class a menu puts on the item for the open page, gives
	// that item a signature of its own.
	plain := linksFrom(t, `<html><body><nav><ul><li class="menu-item"><a href="/x">Item</a></li></ul></nav></body></html>`)
	current := linksFrom(t, `<html><body><nav><ul><li class="menu-item current"><a href="/x">Item</a></li></ul></nav></body></html>`)

	if a, b := plain[0].BlockSignature, current[0].BlockSignature; a == b {
		t.Errorf("the current-page class left the signature unchanged at %d; the documented limit no longer holds", a)
	}
}

func TestNodeDepthSaturatesInsteadOfWrapping(t *testing.T) {
	// Markup nested deeper than a uint16 can count must report the maximum
	// rather than wrap around to a shallow-looking depth.
	root := &html.Node{Type: html.ElementNode, Data: "html"}
	deepest := root
	for i := 0; i < int(^uint16(0))+10; i++ {
		child := &html.Node{Type: html.ElementNode, Data: "div", Parent: deepest}
		deepest = child
	}
	link := &html.Node{Type: html.ElementNode, Data: "a", Parent: deepest}

	if got := nodeDepth(link); got != ^uint16(0) {
		t.Errorf("Depth = %d, want %d", got, ^uint16(0))
	}
}

func TestBlockSignatureSeparatesIDFromClass(t *testing.T) {
	// An id and a class list are different things. Punctuation inside either
	// must not let one be read as the other.
	for _, tc := range []struct{ name, one, two string }{
		{"dot in id", `<div id="a.b">`, `<div id="a" class="b">`},
		{"hash in class", `<div class="x#y">`, `<div id="y" class="x">`},
		{"space-separated classes versus one class", `<div class="a b">`, `<div class="a.b">`},
		// Without the length prefixes the tag name and the id would run
		// together, and these two would sign the same.
		{"tag name running into the id", `<di id="va">`, `<div id="a">`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			one := linksFrom(t, `<html><body>`+tc.one+`<a href="/x">X</a>`)
			two := linksFrom(t, `<html><body>`+tc.two+`<a href="/x">X</a>`)
			if a, b := one[0].BlockSignature, two[0].BlockSignature; a == b {
				t.Errorf("%s and %s both signed %d, want different blocks", tc.one, tc.two, a)
			}
		})
	}
}

func TestBlockSignatureUsesTheID(t *testing.T) {
	// Two otherwise identical wrappers distinguished only by their id are two
	// blocks: many sites name their modules with ids rather than classes.
	one := linksFrom(t, `<html><body><div id="primary"><a href="/x">X</a></div></body></html>`)
	two := linksFrom(t, `<html><body><div id="secondary"><a href="/x">X</a></div></body></html>`)

	if a, b := one[0].BlockSignature, two[0].BlockSignature; a == b {
		t.Errorf("both wrappers signed %d, want different blocks", a)
	}
}

func TestBlockSignatureSeparatesOneAncestorFromTwo(t *testing.T) {
	// One ancestor carrying two classes and two nested ancestors carrying none
	// contribute the same run of parts; only the class count tells them apart.
	one := linksFrom(t, `<html><body><p id="b" class="c d"><a href="/x">X</a></p></body></html>`)
	two := linksFrom(t, `<html><body><p id="b"><c id="d"><a href="/x">X</a></c></p></body></html>`)

	if a, b := one[0].BlockSignature, two[0].BlockSignature; a == b {
		t.Errorf("one ancestor with two classes and two bare ancestors both signed %d", a)
	}
}
