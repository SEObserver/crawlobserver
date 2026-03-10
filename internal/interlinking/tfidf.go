package interlinking

import (
	"math"
	"net/url"
	"sort"
	"sync"

	"github.com/SEObserver/crawlobserver/internal/parser"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

// Document represents a single page's TF-IDF vector (sparse, top-K terms).
type Document struct {
	URL       string
	Title     string
	Lang      string
	PageRank  float64
	WordCount uint32
	TermFreqs map[uint32]float64 // vocab ID → normalized TF-IDF
	Norm      float64            // precomputed L2 norm of TermFreqs
}

// Corpus holds all documents and the shared vocabulary/IDF weights.
type Corpus struct {
	Vocab    map[string]uint32 // term → ID
	IDF      []float64         // ID → IDF value
	Docs     []Document
	DocCount int
}

// maxTermsPerDoc limits per-document terms to control memory usage.
const maxTermsPerDoc = 200

// BuildCorpus constructs a TF-IDF corpus from streamed page HTML rows.
// Workers extract content and tokenize in parallel.
func BuildCorpus(pages <-chan storage.PageHTMLRow, pageInfo map[string]storage.PageMetadata) (*Corpus, error) {
	type docRaw struct {
		url       string
		lang      string
		title     string
		pageRank  float64
		wordCount uint32
		terms     map[string]int // term → raw count
		total     int
	}

	// Phase 1: tokenize all docs in parallel
	const numWorkers = 4
	rawCh := make(chan docRaw, numWorkers*2)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for page := range pages {
				u, _ := url.Parse(page.URL)
				text := parser.ExtractMainContent([]byte(page.HTML), u)
				if len(text) < 50 {
					continue
				}

				meta := pageInfo[page.URL]
				tokens := tokenizeFiltered(text, meta.Lang)
				if len(tokens) == 0 {
					continue
				}

				termCounts := make(map[string]int)
				for _, t := range tokens {
					termCounts[t]++
				}

				rawCh <- docRaw{
					url:       page.URL,
					lang:      meta.Lang,
					title:     meta.Title,
					pageRank:  meta.PageRank,
					wordCount: meta.WordCount,
					terms:     termCounts,
					total:     len(tokens),
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(rawCh)
	}()

	// Phase 2: collect all raw docs, build vocab, compute DF
	var rawDocs []docRaw
	df := make(map[string]int) // term → document frequency
	for rd := range rawCh {
		rawDocs = append(rawDocs, rd)
		for term := range rd.terms {
			df[term]++
		}
	}

	docCount := len(rawDocs)
	if docCount == 0 {
		return &Corpus{}, nil
	}

	// Build vocab (only terms that appear in ≥2 docs and ≤80% of docs)
	vocab := make(map[string]uint32)
	var nextID uint32
	maxDF := int(float64(docCount) * 0.8)
	for term, count := range df {
		if count >= 2 && count <= maxDF {
			vocab[term] = nextID
			nextID++
		}
	}

	// Compute IDF
	idf := make([]float64, nextID)
	for term, id := range vocab {
		idf[id] = math.Log(float64(docCount) / float64(df[term]))
	}

	// Phase 3: build sparse TF-IDF vectors per doc (top-K by TF-IDF weight)
	docs := make([]Document, 0, docCount)
	for _, rd := range rawDocs {
		// Compute TF-IDF for all vocab terms in this doc
		type termWeight struct {
			id     uint32
			weight float64
		}
		var tw []termWeight
		for term, count := range rd.terms {
			id, ok := vocab[term]
			if !ok {
				continue
			}
			tf := float64(count) / float64(rd.total)
			w := tf * idf[id]
			tw = append(tw, termWeight{id, w})
		}

		if len(tw) == 0 {
			continue
		}

		// Keep top-K terms by weight
		sort.Slice(tw, func(i, j int) bool { return tw[i].weight > tw[j].weight })
		if len(tw) > maxTermsPerDoc {
			tw = tw[:maxTermsPerDoc]
		}

		// Build sparse vector and compute L2 norm
		freqs := make(map[uint32]float64, len(tw))
		var normSq float64
		for _, t := range tw {
			freqs[t.id] = t.weight
			normSq += t.weight * t.weight
		}

		docs = append(docs, Document{
			URL:       rd.url,
			Title:     rd.title,
			Lang:      rd.lang,
			PageRank:  rd.pageRank,
			WordCount: rd.wordCount,
			TermFreqs: freqs,
			Norm:      math.Sqrt(normSq),
		})
	}

	return &Corpus{
		Vocab:    vocab,
		IDF:      idf,
		Docs:     docs,
		DocCount: docCount,
	}, nil
}

