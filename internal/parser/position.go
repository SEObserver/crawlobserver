package parser

import (
	"sort"
	"strconv"
	"strings"

	"github.com/SEObserver/crawlobserver/internal/htmlutil"
	"golang.org/x/net/html"
)

// landmarkTags are the HTML5 sectioning elements reported as a link's landmark.
var landmarkTags = map[string]bool{
	"main":    true,
	"article": true,
	"nav":     true,
	"header":  true,
	"footer":  true,
	"aside":   true,
}

// landmarkRoles maps ARIA landmark roles to the element they are equivalent to,
// so that landmarks are always reported with a single vocabulary whether the
// page uses HTML5 elements or ARIA roles.
var landmarkRoles = map[string]string{
	"main":          "main",
	"navigation":    "nav",
	"banner":        "header",
	"contentinfo":   "footer",
	"complementary": "aside",
}

// landmarkOf returns the landmark the link sits in: the nearest ancestor among
// the HTML5 sectioning elements, or, when the page uses none of them, the
// nearest ancestor carrying an equivalent ARIA role. Returns "" when the link
// is in neither, which is common and is reported as it is rather than guessed.
func landmarkOf(n *html.Node) string {
	for e := n.Parent; e != nil && e.Type == html.ElementNode; e = e.Parent {
		if landmarkTags[e.Data] {
			return e.Data
		}
	}
	for e := n.Parent; e != nil && e.Type == html.ElementNode; e = e.Parent {
		role, ok := htmlutil.NodeAttr(e, "role")
		if !ok {
			continue
		}
		// A role attribute holds a list of fallback roles; the first one that
		// is a landmark wins, as in the ARIA spec.
		for _, token := range strings.Fields(role) {
			if tag, ok := landmarkRoles[strings.ToLower(token)]; ok {
				return tag
			}
		}
	}
	return ""
}

// nodeXPath returns the absolute XPath of an element, e.g. /html/body/nav/ul/li[2]/a.
// A positional predicate is added only when the element has siblings of the same
// name, which is how browsers and crawlers usually spell these paths.
func nodeXPath(n *html.Node) string {
	return nodeXPathWithCache(n, nil)
}

// nodeXPathWithCache returns an absolute XPath while reusing the sibling
// indexes collected for each parent. Keeping nodeXPath as a small wrapper
// preserves its useful standalone behavior for callers and tests that do not
// already have a page-level cache.
func nodeXPathWithCache(n *html.Node, cache *xpathIndexCache) string {
	var segments []string
	for e := n; e != nil && e.Type == html.ElementNode; e = e.Parent {
		segments = append(segments, xpathSegmentWithCache(e, cache))
	}

	var b strings.Builder
	for i := len(segments) - 1; i >= 0; i-- {
		b.WriteByte('/')
		b.WriteString(segments[i])
	}
	return b.String()
}

func xpathSegment(e *html.Node) string {
	return xpathSegmentWithCache(e, nil)
}

// xpathIndexCache stores same-named sibling positions for parents already
// encountered while producing paths. Computing a parent's complete index once
// keeps a long run of links under one list linear instead of repeatedly
// scanning the list for every link.
type xpathIndexCache struct {
	parents map[*html.Node]xpathParentIndex
}

type xpathParentIndex struct {
	positions map[*html.Node]int
	counts    map[string]int
}

func newXPathIndexCache() *xpathIndexCache {
	return &xpathIndexCache{parents: make(map[*html.Node]xpathParentIndex)}
}

func (c *xpathIndexCache) parentIndex(parent *html.Node) xpathParentIndex {
	if index, ok := c.parents[parent]; ok {
		return index
	}

	index := xpathParentIndex{
		positions: make(map[*html.Node]int),
		counts:    make(map[string]int),
	}
	for sibling := parent.FirstChild; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type != html.ElementNode {
			continue
		}
		index.counts[sibling.Data]++
		index.positions[sibling] = index.counts[sibling.Data]
	}
	c.parents[parent] = index
	return index
}

func xpathSegmentWithCache(e *html.Node, cache *xpathIndexCache) string {
	if e.Parent == nil {
		return e.Data
	}
	if cache != nil {
		index := cache.parentIndex(e.Parent)
		if index.counts[e.Data] <= 1 {
			return e.Data
		}
		return e.Data + "[" + strconv.Itoa(index.positions[e]) + "]"
	}

	position, count := 0, 0
	for s := e.Parent.FirstChild; s != nil; s = s.NextSibling {
		if s.Type != html.ElementNode || s.Data != e.Data {
			continue
		}
		count++
		if s == e {
			position = count
		}
	}
	if count <= 1 {
		return e.Data
	}
	return e.Data + "[" + strconv.Itoa(position) + "]"
}

// nodeDepth counts the element ancestors above n. In a document parsed from a
// complete page, a link sitting directly in the body has depth 2 (html, body).
func nodeDepth(n *html.Node) uint16 {
	depth := 0
	for e := n.Parent; e != nil && e.Type == html.ElementNode; e = e.Parent {
		depth++
	}
	if depth > int(^uint16(0)) {
		return ^uint16(0)
	}
	return uint16(depth)
}

// blockSignature identifies the block a link sits in, so that a block repeated
// across pages — a menu, a footer, a generated "related items" module — can be
// recognised as one block rather than as thousands of unrelated links.
//
// It hashes the chain of element ancestors between the body and the link,
// each contributing its tag name, its id and its class list. Two links whose
// surrounding markup comes from the same template therefore share a signature,
// and links in differently-built blocks do not. Sibling positions are left out
// on purpose: every item of one menu belongs to the same block. The exact
// position is in the XPath for callers that need to tell items apart.
//
// Attributes on <html> and <body> are excluded because many CMSes stamp
// per-page values there (a post id, a page slug). Including them would give
// every page its own signatures and hide the very repetition this measures.
//
// Two limits worth knowing: markup that changes with the current page still
// splits a block — a menu item carrying a "current" class on its own page gets
// its own signature — and a site whose class names are generated per page
// cannot be grouped at all.
func blockSignature(n *html.Node) uint64 {
	var chain []*html.Node
	for e := n.Parent; e != nil && e.Type == html.ElementNode; e = e.Parent {
		if e.Data == "body" || e.Data == "html" {
			break
		}
		chain = append(chain, e)
	}

	var b strings.Builder
	for i := len(chain) - 1; i >= 0; i-- {
		writeSignatureStep(&b, chain[i])
	}
	return hashToken(b.String())
}

// writeSignatureStep writes one ancestor: its tag name, its id, then its class
// list. Classes are sorted so that the same block written with its classes in
// another order still signs the same.
//
// Every part is length-prefixed because ids and class names may contain any
// character, punctuation included. Spelling a step as "tag#id.class" would let
// an element with the single id "a.b" sign exactly like one with the id "a" and
// the class "b", quietly merging two different blocks.
func writeSignatureStep(b *strings.Builder, e *html.Node) {
	writeSignaturePart(b, e.Data)

	id, _ := htmlutil.NodeAttr(e, "id")
	writeSignaturePart(b, id)

	class, _ := htmlutil.NodeAttr(e, "class")
	classes := strings.Fields(class)
	sort.Strings(classes)
	writeSignaturePart(b, strconv.Itoa(len(classes)))
	for _, c := range classes {
		writeSignaturePart(b, c)
	}
}

// writeSignaturePart writes one variable-length component as "<length>:<value>",
// so that a run of components can only be read one way.
func writeSignaturePart(b *strings.Builder, s string) {
	b.WriteString(strconv.Itoa(len(s)))
	b.WriteByte(':')
	b.WriteString(s)
}
