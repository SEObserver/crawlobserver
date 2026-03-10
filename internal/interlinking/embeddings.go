package interlinking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
)

// EmbeddingProvider generates vector embeddings from text.
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

// OpenAIProvider calls the OpenAI embeddings API.
type OpenAIProvider struct {
	APIKey    string
	Model     string
	BatchSize int
}

type openAIEmbRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openAIEmbResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// NewOpenAIProvider creates a new OpenAI embedding provider.
func NewOpenAIProvider(apiKey, model string, batchSize int) *OpenAIProvider {
	if model == "" {
		model = "text-embedding-3-small"
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	return &OpenAIProvider{
		APIKey:    apiKey,
		Model:     model,
		BatchSize: batchSize,
	}
}

// Dimension returns the embedding dimension for the configured model.
func (p *OpenAIProvider) Dimension() int {
	switch p.Model {
	case "text-embedding-3-large":
		return 3072
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-ada-002":
		return 1536
	default:
		return 1536
	}
}

// Embed generates embeddings for a batch of texts.
// Automatically chunks into BatchSize sub-batches.
func (p *OpenAIProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	result := make([][]float32, len(texts))

	for i := 0; i < len(texts); i += p.BatchSize {
		end := i + p.BatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		embeddings, err := p.embedBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("embedding batch %d-%d: %w", i, end, err)
		}

		for j, emb := range embeddings {
			result[i+j] = emb
		}
	}

	return result, nil
}

func (p *OpenAIProvider) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(openAIEmbRequest{
		Input: texts,
		Model: p.Model,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result openAIEmbResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	embeddings := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < len(embeddings) {
			embeddings[d.Index] = d.Embedding
		}
	}

	return embeddings, nil
}

// CosineDistanceFloat32 computes cosine distance between two float32 vectors.
func CosineDistanceFloat32(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 1.0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 1.0
	}
	return 1.0 - (dot / denom)
}
