package constants

import (
	"fmt"
	"time"
)

// =============================================================================
// PRODUCT RECOMMENDATION CONFIGURATION
// =============================================================================

// Product Search Configuration
const (
	// QdrantSearchSize is the number of products to fetch from Qdrant vector search
	QdrantSearchSize = 60

	// Batch sizes for progressive database queries (ratio 15:20:25)
	BatchSize1 = 15
	BatchSize2 = 20
	BatchSize3 = 25

	// SpatialBatchSize is kept for backward compatibility (equals BatchSize1)
	SpatialBatchSize = BatchSize1

	// MinProductsToShow is the minimum number of unique products needed before querying next batch
	MinProductsToShow = 5

	// MaxProductsToShow is the maximum number of unique products to return in response
	MaxProductsToShow = 10
)

// Fallback Search Configuration
const (
	// FallbackTier1Miles is the first fallback radius to try (in miles)
	FallbackTier1Miles = 100

	// FallbackTier2Miles is the second fallback radius to try (in miles)
	FallbackTier2Miles = 200
)

// GetFallbackTiers returns the ordered list of fallback radius tiers to try
func GetFallbackTiers() []float64 {
	return []float64{FallbackTier1Miles, FallbackTier2Miles}
}

// =============================================================================
// PRODUCT RECOMMENDATION MESSAGES
// =============================================================================

// Message templates for all product recommendation scenarios
const (
	// MessageFoundWithinRadius is used when products are found within user's original radius
	// Placeholders: {count}, {product_type}, {radius}
	MessageFoundWithinRadius = "I found %d great options for %s within your %.0f-mile area!"

	// MessageFoundAtFallbackRadius is used when products are found at a fallback radius
	// Placeholders: {product_type}, {original_radius}, {count}, {fallback_radius}
	MessageFoundAtFallbackRadius = "We didn't find any %s within your %.0f-mile area, but we found %d great options within %.0f miles!"

	// MessageNoProductsWithin200Miles is used when no products found even at maximum fallback radius
	// Placeholders: {product_type}
	MessageNoProductsWithin200Miles = "We found matches for %s in our catalog, but unfortunately none are available within 200 miles of your location. Cannabis availability varies by region—check back later as inventory changes frequently."
)

// GenerateFallbackFoundMessage creates a message when products are found at a fallback radius
func GenerateFallbackFoundMessage(
	productQuery string,
	originalRadius float64,
	productCount int,
	fallbackRadius float64,
) string {
	return fmt.Sprintf(
		MessageFoundAtFallbackRadius,
		productQuery,
		originalRadius,
		productCount,
		fallbackRadius,
	)
}

// GenerateNoProductsMessage creates a message when no products found even at max fallback
func GenerateNoProductsMessage(productQuery string) string {
	return fmt.Sprintf(MessageNoProductsWithin200Miles, productQuery)
}

// GenerateSuccessMessage creates a message when products are found within user's radius
func GenerateSuccessMessage(
	productCount int,
	productQuery string,
	userRadius float64,
) string {
	return fmt.Sprintf(
		MessageFoundWithinRadius,
		productCount,
		productQuery,
		userRadius,
	)
}

// =============================================================================
// WEBSOCKET TIMEOUT CONFIGURATION
// =============================================================================

// WebSocket Timeout Configuration
const (
	// WSAIResponseTimeout is the timeout for AI response generation
	WSAIResponseTimeout = 300 * time.Second

	// WSProductRecommendationTimeout is the timeout for product recommendation analysis
	WSProductRecommendationTimeout = 300 * time.Second

	// WSKnowledgeRetrievalTimeout is the timeout for knowledge retrieval
	WSKnowledgeRetrievalTimeout = 30 * time.Second

	// WSSuggestionAnalysisTimeout is the timeout for suggestion analysis
	WSSuggestionAnalysisTimeout = 15 * time.Second

	// WSDefaultTimeout is the default timeout for general operations
	WSDefaultTimeout = 60 * time.Second
)

// GetProductSearchSize returns the Qdrant search size (for backward compatibility)
func GetProductSearchSize() int {
	return QdrantSearchSize
}

// GetSpatialBatchSize returns the spatial batch size (for backward compatibility)
func GetSpatialBatchSize() int {
	return SpatialBatchSize
}

// GetMinProductsToShow returns the minimum products to show (for backward compatibility)
func GetMinProductsToShow() int {
	return MinProductsToShow
}

// GetMaxProductsToShow returns the maximum products to show in response
func GetMaxProductsToShow() int {
	return MaxProductsToShow
}

// GetBatchSizes returns the ordered list of batch sizes for progressive querying
func GetBatchSizes() []int {
	return []int{BatchSize1, BatchSize2, BatchSize3}
}

// GetBatchRanges returns the start and end indices for each batch based on batch sizes
// Returns a slice of [start, end] pairs for slicing product IDs
func GetBatchRanges(totalProducts int) [][]int {
	batchSizes := GetBatchSizes()
	var ranges [][]int
	start := 0

	for _, size := range batchSizes {
		if start >= totalProducts {
			break
		}
		end := start + size
		if end > totalProducts {
			end = totalProducts
		}
		ranges = append(ranges, []int{start, end})
		start = end
	}

	return ranges
}

// =============================================================================
// OPENSEARCH CONFIGURATION (AWS OpenSearch Serverless)
// =============================================================================

// OpenSearch Configuration Constants
const (
	// OpenSearchDefaultIndex is the default index name for cannabis educational knowledge
	OpenSearchDefaultIndex = "cannabis-educational-knowledge"

	// OpenSearchDefaultRegion is the default AWS region
	OpenSearchDefaultRegion = "us-east-1"

	// OpenSearchSearchSize is the number of documents to fetch from OpenSearch vector search
	OpenSearchSearchSize = 10

	// OpenSearchScoreThreshold is the minimum similarity score to consider a result relevant
	// Results below this threshold trigger the fallback search
	OpenSearchScoreThreshold = 0.75

	// OpenSearchTimeout is the timeout for OpenSearch requests
	OpenSearchTimeout = 30 * time.Second
)

// Knowledge Retrieval Configuration
const (
	// KnowledgeFallbackEnabled controls whether fallback to external search is enabled
	KnowledgeFallbackEnabled = true

	// KnowledgeMaxExternalResults is the max number of external search results to process
	KnowledgeMaxExternalResults = 5

	// KnowledgeCacheTTL is the default TTL for cached knowledge (0 = no TTL)
	KnowledgeCacheTTL = 0 * time.Hour
)

// Source Type Constants for knowledge documents
const (
	SourceTypeDuckDuckGo    = "duckduckgo"
	SourceTypePubMed        = "pubmed"
	SourceTypeUserGenerated = "user_generated"
	SourceTypeBrand         = "brand"
)

// Content Type Constants for knowledge documents
const (
	ContentTypeEducational = "educational"
	ContentTypeResearch    = "research"
	ContentTypeProductInfo = "product_info"
)

// OpenSearch Environment Variable Names
const (
	EnvOpenSearchEndpoint = "OPENSEARCH_ENDPOINT"
	EnvOpenSearchIndex    = "OPENSEARCH_INDEX"
	EnvOpenSearchRegion   = "OPENSEARCH_REGION"
	EnvOpenSearchEnabled  = "OPENSEARCH_ENABLED"
)

// GetOpenSearchSearchSize returns the OpenSearch search size
func GetOpenSearchSearchSize() int {
	return OpenSearchSearchSize
}

// GetOpenSearchScoreThreshold returns the minimum relevance score threshold
func GetOpenSearchScoreThreshold() float64 {
	return OpenSearchScoreThreshold
}

// IsKnowledgeFallbackEnabled returns whether fallback search is enabled
func IsKnowledgeFallbackEnabled() bool {
	return KnowledgeFallbackEnabled
}
