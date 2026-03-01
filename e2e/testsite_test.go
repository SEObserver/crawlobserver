//go:build integration

package e2e

import (
	"fmt"
	"net/http"
)

// testSiteHandler returns an http.Handler serving a deterministic mini-website
// with ~15 interconnected pages for E2E crawl testing.
func testSiteHandler() http.Handler {
	mux := http.NewServeMux()

	page := func(title, body string) []byte {
		return []byte(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>%s</title>
<meta name="description" content="Test page: %s"></head>
<body><h1>%s</h1>%s</body>
</html>`, title, title, title, body))
	}

	// Homepage
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(404)
			w.Write(page("Not Found", `<p>Page not found.</p><p><a href="/">Home</a></p>`))
			return
		}
		w.Write(page("Home", `
<nav>
  <a href="/products">Products</a>
  <a href="/blog">Blog</a>
  <a href="/about">About</a>
  <a href="/redirect">Redirect Link</a>
  <a href="/redirect-chain">Redirect Chain</a>
  <a href="/gone">Gone Page</a>
</nav>
<p>Welcome to the test site.</p>`))
	})

	// Products listing
	mux.HandleFunc("/products", func(w http.ResponseWriter, r *http.Request) {
		w.Write(page("Products", `
<ul>
  <li><a href="/products/widget">Widget</a></li>
  <li><a href="/products/gadget">Gadget</a></li>
</ul>
<a href="/">Home</a>`))
	})

	// Product: widget (noindex)
	mux.HandleFunc("/products/widget", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Widget</title>
<meta name="robots" content="noindex">
<meta name="description" content="The amazing widget"></head>
<body><h1>Widget</h1>
<p>This product is noindexed.</p>
<a href="/products">Back to products</a>
<a href="/products/gadget">See also: Gadget</a>
</body></html>`))
	})

	// Product: gadget
	mux.HandleFunc("/products/gadget", func(w http.ResponseWriter, r *http.Request) {
		w.Write(page("Gadget", `
<p>The best gadget ever.</p>
<a href="/products">Back to products</a>
<a href="/products/widget">See also: Widget</a>`))
	})

	// Blog listing
	mux.HandleFunc("/blog", func(w http.ResponseWriter, r *http.Request) {
		w.Write(page("Blog", `
<ul>
  <li><a href="/blog/post-1">Post 1: SEO Tips</a></li>
  <li><a href="/blog/post-2">Post 2: Crawling Best Practices</a></li>
</ul>
<a href="/">Home</a>`))
	})

	// Blog post 1
	mux.HandleFunc("/blog/post-1", func(w http.ResponseWriter, r *http.Request) {
		w.Write(page("SEO Tips", `
<p>Here are some SEO tips for your website.</p>
<a href="/blog">Blog</a>
<a href="/blog/post-2">Next: Crawling Best Practices</a>`))
	})

	// Blog post 2
	mux.HandleFunc("/blog/post-2", func(w http.ResponseWriter, r *http.Request) {
		w.Write(page("Crawling Best Practices", `
<p>Best practices for web crawling.</p>
<a href="/blog">Blog</a>
<a href="/blog/post-1">Previous: SEO Tips</a>`))
	})

	// About page with wrong canonical
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>About Us</title>
<link rel="canonical" href="/about-us">
<meta name="description" content="About our company"></head>
<body><h1>About Us</h1>
<p>We build crawlers.</p>
<a href="/">Home</a>
</body></html>`))
	})

	// Redirect 301 -> /products
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/products", http.StatusMovedPermanently)
	})

	// Redirect chain: 301 -> /redirect -> /products
	mux.HandleFunc("/redirect-chain", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/redirect", http.StatusMovedPermanently)
	})

	// 404 page
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write(page("Gone", `<p>This page no longer exists.</p><a href="/">Home</a>`))
	})

	// 500 page
	mux.HandleFunc("/server-error", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	})

	// Private page (should be blocked by robots.txt)
	mux.HandleFunc("/private/secret", func(w http.ResponseWriter, r *http.Request) {
		w.Write(page("Secret", `<p>This page is private.</p><a href="/">Home</a>`))
	})

	// robots.txt
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("User-agent: *\nDisallow: /private/\n"))
	})

	// sitemap.xml
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>/</loc></url>
  <url><loc>/products</loc></url>
  <url><loc>/products/gadget</loc></url>
  <url><loc>/blog</loc></url>
  <url><loc>/blog/post-1</loc></url>
  <url><loc>/blog/post-2</loc></url>
  <url><loc>/about</loc></url>
</urlset>`))
	})

	return mux
}
