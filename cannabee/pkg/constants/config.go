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
	QdrantSearchSize = 20

	// SpatialBatchSize is the number of product IDs to query per database batch
	SpatialBatchSize = 10

	// MinProductsToShow is the minimum number of unique products needed before skipping second batch
	MinProductsToShow = 5
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
