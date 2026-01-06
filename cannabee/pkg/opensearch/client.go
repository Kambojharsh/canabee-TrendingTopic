package opensearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// Client represents an AWS OpenSearch Serverless client
type Client struct {
	endpoint   string
	index      string
	region     string
	httpClient *http.Client
	awsCreds   aws.CredentialsProvider
	enabled    bool
}

// Config holds configuration for the OpenSearch client
type Config struct {
	Endpoint        string // AWS OpenSearch Serverless endpoint (e.g., https://xxx.us-east-1.aoss.amazonaws.com)
	Index           string // Index name for cannabis knowledge
	Region          string // AWS region
	AccessKeyID     string // AWS access key (optional, uses default chain if empty)
	SecretAccessKey string // AWS secret key (optional, uses default chain if empty)
	Enabled         bool   // Whether OpenSearch is enabled
}

// NewClient creates a new OpenSearch client
func NewClient(cfg Config) (*Client, error) {
	log.Printf("=== Initializing OpenSearch Client ===")
	log.Printf("Endpoint: %s", cfg.Endpoint)
	log.Printf("Index: %s", cfg.Index)
	log.Printf("Region: %s", cfg.Region)
	log.Printf("Enabled: %t", cfg.Enabled)

	if !cfg.Enabled {
		log.Printf("OpenSearch is disabled - client will return empty results")
		return &Client{
			endpoint: cfg.Endpoint,
			index:    cfg.Index,
			region:   cfg.Region,
			enabled:  false,
		}, nil
	}

	// Validate required configuration
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("OpenSearch endpoint is required")
	}
	if cfg.Index == "" {
		return nil, fmt.Errorf("OpenSearch index name is required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1" // Default region
	}

	// Set up AWS credentials
	var awsCreds aws.CredentialsProvider
	ctx := context.Background()

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		// Use explicit credentials if provided
		awsCreds = credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")
		log.Printf("Using explicit AWS credentials")
	} else {
		// Use default credential chain (env vars, IAM role, etc.)
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w", err)
		}
		awsCreds = awsCfg.Credentials
		log.Printf("Using default AWS credential chain")
	}

	client := &Client{
		endpoint:   strings.TrimSuffix(cfg.Endpoint, "/"),
		index:      cfg.Index,
		region:     cfg.Region,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		awsCreds:   awsCreds,
		enabled:    true,
	}

	log.Printf("OpenSearch client initialized successfully")
	return client, nil
}

// NewClientFromEnv creates a new OpenSearch client from environment variables
func NewClientFromEnv() (*Client, error) {
	enabled := os.Getenv("OPENSEARCH_ENABLED") == "true"

	cfg := Config{
		Endpoint:        os.Getenv("OPENSEARCH_ENDPOINT"),
		Index:           getEnvOrDefault("OPENSEARCH_INDEX", "cannabis-educational-knowledge"),
		Region:          getEnvOrDefault("OPENSEARCH_REGION", getEnvOrDefault("AWS_REGION", "us-east-1")),
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		Enabled:         enabled,
	}

	return NewClient(cfg)
}

// IsEnabled returns whether the OpenSearch client is enabled
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// signRequest signs an HTTP request with AWS Signature V4
func (c *Client) signRequest(ctx context.Context, req *http.Request, body []byte) error {
	creds, err := c.awsCreds.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve AWS credentials: %w", err)
	}

	// Calculate body hash
	hash := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(hash[:])

	signer := v4.NewSigner()
	err = signer.SignHTTP(ctx, creds, req, payloadHash, "aoss", c.region, time.Now())
	if err != nil {
		return fmt.Errorf("failed to sign request: %w", err)
	}

	return nil
}

// doRequest executes a signed HTTP request to OpenSearch
func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if !c.enabled {
		return nil, fmt.Errorf("OpenSearch client is not enabled")
	}

	url := fmt.Sprintf("%s/%s", c.endpoint, path)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Sign the request
	if err := c.signRequest(ctx, req, body); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("OpenSearch error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Search performs a vector similarity search with optional metadata filters
func (c *Client) Search(ctx context.Context, queryVector []float32, filters *SearchFilters, topK int) (*SearchResponse, error) {
	if !c.enabled {
		log.Printf("OpenSearch disabled - returning empty search results")
		return &SearchResponse{Results: []SearchResult{}, TotalHits: 0}, nil
	}

	log.Printf("=== OpenSearch Search ===")
	log.Printf("Vector dimensions: %d", len(queryVector))
	log.Printf("Top K: %d", topK)

	if topK <= 0 {
		topK = 10
	}

	// Build the k-NN query
	query := map[string]interface{}{
		"size": topK,
		"query": map[string]interface{}{
			"knn": map[string]interface{}{
				"embedding_vector": map[string]interface{}{
					"vector": queryVector,
					"k":      topK,
				},
			},
		},
	}

	// Add filters if provided
	if filters != nil {
		filterClauses := c.buildFilterClauses(filters)
		if len(filterClauses) > 0 {
			query["query"] = map[string]interface{}{
				"bool": map[string]interface{}{
					"must": map[string]interface{}{
						"knn": map[string]interface{}{
							"embedding_vector": map[string]interface{}{
								"vector": queryVector,
								"k":      topK,
							},
						},
					},
					"filter": filterClauses,
				},
			}
		}
	}

	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	log.Printf("Search query: %s", string(queryBytes))

	path := fmt.Sprintf("%s/_search", c.index)
	respBody, err := c.doRequest(ctx, "POST", path, queryBytes)
	if err != nil {
		return nil, err
	}

	// Parse response
	var osResp struct {
		Took int64 `json:"took"`
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				ID     string            `json:"_id"`
				Score  float64           `json:"_score"`
				Source KnowledgeDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(respBody, &osResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	results := make([]SearchResult, len(osResp.Hits.Hits))
	for i, hit := range osResp.Hits.Hits {
		results[i] = SearchResult{
			Document: hit.Source,
			Score:    hit.Score,
			ID:       hit.ID,
		}
	}

	log.Printf("Search returned %d results (total: %d, took: %dms)",
		len(results), osResp.Hits.Total.Value, osResp.Took)

	return &SearchResponse{
		Results:   results,
		TotalHits: osResp.Hits.Total.Value,
		TookMs:    osResp.Took,
	}, nil
}

// buildFilterClauses builds OpenSearch filter clauses from SearchFilters
func (c *Client) buildFilterClauses(filters *SearchFilters) []map[string]interface{} {
	var clauses []map[string]interface{}

	if filters.SourceType != "" {
		clauses = append(clauses, map[string]interface{}{
			"term": map[string]interface{}{"source_type": filters.SourceType},
		})
	}

	if filters.ExcludePubMed {
		clauses = append(clauses, map[string]interface{}{
			"bool": map[string]interface{}{
				"must_not": map[string]interface{}{
					"term": map[string]interface{}{"source_type": SourceTypePubMed},
				},
			},
		})
	}

	if filters.ContentType != "" {
		clauses = append(clauses, map[string]interface{}{
			"term": map[string]interface{}{"content_type": filters.ContentType},
		})
	}

	if filters.Category != "" {
		clauses = append(clauses, map[string]interface{}{
			"term": map[string]interface{}{"category": filters.Category},
		})
	}

	if filters.StrainType != "" {
		clauses = append(clauses, map[string]interface{}{
			"term": map[string]interface{}{"strain_type": filters.StrainType},
		})
	}

	if filters.THCLevel != "" {
		clauses = append(clauses, map[string]interface{}{
			"term": map[string]interface{}{"thc_level": filters.THCLevel},
		})
	}

	if filters.CBDLevel != "" {
		clauses = append(clauses, map[string]interface{}{
			"term": map[string]interface{}{"cbd_level": filters.CBDLevel},
		})
	}

	if filters.TimeOfUse != "" {
		clauses = append(clauses, map[string]interface{}{
			"term": map[string]interface{}{"time_of_use": filters.TimeOfUse},
		})
	}

	if len(filters.PrimaryAilments) > 0 {
		clauses = append(clauses, map[string]interface{}{
			"terms": map[string]interface{}{"primary_ailments": filters.PrimaryAilments},
		})
	}

	if len(filters.PrimaryEffects) > 0 {
		clauses = append(clauses, map[string]interface{}{
			"terms": map[string]interface{}{"primary_effects": filters.PrimaryEffects},
		})
	}

	if len(filters.ConsumptionMethod) > 0 {
		clauses = append(clauses, map[string]interface{}{
			"terms": map[string]interface{}{"consumption_method": filters.ConsumptionMethod},
		})
	}

	if len(filters.ExperienceLevel) > 0 {
		clauses = append(clauses, map[string]interface{}{
			"terms": map[string]interface{}{"experience_level": filters.ExperienceLevel},
		})
	}

	return clauses
}

// Upsert inserts or updates a knowledge document
func (c *Client) Upsert(ctx context.Context, doc KnowledgeDocument) error {
	if !c.enabled {
		log.Printf("OpenSearch disabled - skipping upsert for hash: %s", doc.ContentHash)
		return nil
	}

	log.Printf("=== OpenSearch Upsert ===")
	log.Printf("Content hash: %s", doc.ContentHash)
	log.Printf("Source type: %s", doc.SourceType)

	// Set timestamps
	now := time.Now()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = now
	}
	doc.UpdatedAt = now

	docBytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	// Use content_hash as document ID for deduplication
	path := fmt.Sprintf("%s/_doc/%s", c.index, doc.ContentHash)
	_, err = c.doRequest(ctx, "PUT", path, docBytes)
	if err != nil {
		return err
	}

	log.Printf("Document upserted successfully: %s", doc.ContentHash)
	return nil
}

// BulkUpsert inserts or updates multiple documents in a single request
func (c *Client) BulkUpsert(ctx context.Context, docs []KnowledgeDocument) (*BulkResponse, error) {
	if !c.enabled {
		log.Printf("OpenSearch disabled - skipping bulk upsert of %d documents", len(docs))
		return &BulkResponse{Succeeded: 0, Failed: 0}, nil
	}

	if len(docs) == 0 {
		return &BulkResponse{Succeeded: 0, Failed: 0}, nil
	}

	log.Printf("=== OpenSearch Bulk Upsert ===")
	log.Printf("Documents to upsert: %d", len(docs))

	// Build bulk request body (NDJSON format)
	var buffer bytes.Buffer
	now := time.Now()

	for _, doc := range docs {
		// Set timestamps
		if doc.CreatedAt.IsZero() {
			doc.CreatedAt = now
		}
		doc.UpdatedAt = now

		// Action line
		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": c.index,
				"_id":    doc.ContentHash,
			},
		}
		actionBytes, _ := json.Marshal(action)
		buffer.Write(actionBytes)
		buffer.WriteByte('\n')

		// Document line
		docBytes, _ := json.Marshal(doc)
		buffer.Write(docBytes)
		buffer.WriteByte('\n')
	}

	respBody, err := c.doRequest(ctx, "POST", "_bulk", buffer.Bytes())
	if err != nil {
		return nil, err
	}

	// Parse bulk response
	var bulkResp struct {
		Took   int64 `json:"took"`
		Errors bool  `json:"errors"`
		Items  []struct {
			Index struct {
				ID     string `json:"_id"`
				Status int    `json:"status"`
				Error  *struct {
					Type   string `json:"type"`
					Reason string `json:"reason"`
				} `json:"error,omitempty"`
			} `json:"index"`
		} `json:"items"`
	}

	if err := json.Unmarshal(respBody, &bulkResp); err != nil {
		return nil, fmt.Errorf("failed to parse bulk response: %w", err)
	}

	succeeded := 0
	failed := 0
	var items []BulkItemResponse

	for _, item := range bulkResp.Items {
		itemResp := BulkItemResponse{
			ID:     item.Index.ID,
			Status: item.Index.Status,
		}
		if item.Index.Error != nil {
			itemResp.Error = fmt.Sprintf("%s: %s", item.Index.Error.Type, item.Index.Error.Reason)
			failed++
		} else {
			succeeded++
		}
		items = append(items, itemResp)
	}

	log.Printf("Bulk upsert completed: %d succeeded, %d failed (took: %dms)",
		succeeded, failed, bulkResp.Took)

	return &BulkResponse{
		Took:      bulkResp.Took,
		Errors:    bulkResp.Errors,
		Succeeded: succeeded,
		Failed:    failed,
		Items:     items,
	}, nil
}

// Exists checks if a document with the given content hash already exists
func (c *Client) Exists(ctx context.Context, contentHash string) (bool, error) {
	if !c.enabled {
		return false, nil
	}

	path := fmt.Sprintf("%s/_doc/%s", c.index, contentHash)

	url := fmt.Sprintf("%s/%s", c.endpoint, path)
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	if err := c.signRequest(ctx, req, nil); err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// 200 = exists, 404 = does not exist
	return resp.StatusCode == 200, nil
}

// Delete removes a document by content hash
func (c *Client) Delete(ctx context.Context, contentHash string) error {
	if !c.enabled {
		return nil
	}

	path := fmt.Sprintf("%s/_doc/%s", c.index, contentHash)
	_, err := c.doRequest(ctx, "DELETE", path, nil)
	return err
}

// Health checks the health status of the OpenSearch cluster
func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	if !c.enabled {
		return &HealthStatus{
			Status:    "disabled",
			Available: false,
			Message:   "OpenSearch client is not enabled",
		}, nil
	}

	respBody, err := c.doRequest(ctx, "GET", "_cluster/health", nil)
	if err != nil {
		return &HealthStatus{
			Status:    "red",
			Available: false,
			Message:   err.Error(),
		}, nil
	}

	var health struct {
		ClusterName string `json:"cluster_name"`
		Status      string `json:"status"`
	}

	if err := json.Unmarshal(respBody, &health); err != nil {
		return nil, fmt.Errorf("failed to parse health response: %w", err)
	}

	return &HealthStatus{
		Status:      health.Status,
		ClusterName: health.ClusterName,
		Available:   true,
	}, nil
}

// GetIndex returns the index name
func (c *Client) GetIndex() string {
	return c.index
}

// --- Utility Functions ---

// NormalizeTextForEmbedding prepares text for embedding generation
// Applies: lowercase, trim, collapse whitespace, remove special chars
func NormalizeTextForEmbedding(text string) string {
	// Trim leading/trailing whitespace
	text = strings.TrimSpace(text)

	// Convert to lowercase
	text = strings.ToLower(text)

	// Collapse multiple whitespace to single space
	spaceRegex := regexp.MustCompile(`\s+`)
	text = spaceRegex.ReplaceAllString(text, " ")

	return text
}

// HashContent generates SHA256 hash of normalized text for deduplication
func HashContent(normalizedText string) string {
	hash := sha256.Sum256([]byte(normalizedText))
	return hex.EncodeToString(hash[:])
}

// Helper function for environment variables
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
