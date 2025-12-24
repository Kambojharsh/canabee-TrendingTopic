package products

import (
	"cannabee/pkg/chat"
	"cannabee/pkg/constants"
	"cannabee/pkg/db"
	"cannabee/pkg/qdrant"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// ProductRecommendationService handles product recommendations using vector search
type ProductRecommendationService struct {
	qdrantClient *qdrant.Client
	openaiClient *chat.OpenAIClient
	db           *sql.DB
	collection   string
}

// Config holds configuration for the product recommendation service
type Config struct {
	QdrantHost       string
	QdrantPort       int
	QdrantAPIKey     string
	QdrantUseTLS     bool
	QdrantCollection string
}

// ProductInfo represents the product information returned to frontend
type ProductInfo struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Price           float64  `json:"price"`
	ImageURL        string   `json:"image_url,omitempty"`
	Category        string   `json:"category,omitempty"`
	THCContent      *float64 `json:"thc_content,omitempty"`
	CBDContent      *float64 `json:"cbd_content,omitempty"`
	StrainType      string   `json:"strain_type,omitempty"`
	ListingURL      string   `json:"listing_url,omitempty"`
	RetailerID      *int64   `json:"retailer_id,omitempty"`
	ProductID       *int64   `json:"product_id,omitempty"`
	InventoryCount  int32    `json:"inventory_count,omitempty"`
	DiscountedPrice *float64 `json:"discounted_price,omitempty"`
	// NEW: Location-related fields
	RetailerName    string   `json:"retailer_name,omitempty"`
	DistanceMile    *float64 `json:"distance_mile,omitempty"`
	RetailerLat     *float64 `json:"retailer_latitude,omitempty"`
	RetailerLng     *float64 `json:"retailer_longitude,omitempty"`
	RetailerHomeURL string   `json:"retailer_home_url,omitempty"`
	RetailerMenuURL string   `json:"retailer_menu_url,omitempty"`
	// NEW: AI-generated product insights
	WhyThisProduct string   `json:"why_this_product,omitempty"`
	TopFeelings    []string `json:"top_feelings,omitempty"`
	KeyInfo        *KeyInfo `json:"key_info,omitempty"`
	Ingredients    string   `json:"ingredients,omitempty"`
}

// KeyInfo represents structured key information about a product
type KeyInfo struct {
	Flavor            string `json:"flavor,omitempty"`
	THCCBDContent     string `json:"thc_cbd_content,omitempty"`
	StrainType        string `json:"strain_type,omitempty"`
	TypicalTimeOfUse  string `json:"typical_time_of_use,omitempty"`
	ExpectedIntensity string `json:"expected_intensity,omitempty"`
	ConsumptionFormat string `json:"consumption_format,omitempty"`
}

// UserLocation represents user's location for filtering products
type UserLocation struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	RadiusMile float64 `json:"radius_mile"`
	Address    string  `json:"address,omitempty"`
}

// ProductRecommendationResponse represents the response containing product recommendations
type ProductRecommendationResponse struct {
	ShouldShowRecommendations bool          `json:"should_show_recommendations"`
	Products                  []ProductInfo `json:"products"`
	Query                     string        `json:"query"`
	Reason                    string        `json:"reason"`
}

// NewProductRecommendationService creates a new product recommendation service
func NewProductRecommendationService(openaiClient *chat.OpenAIClient, database *sql.DB) (*ProductRecommendationService, error) {
	log.Printf("=== Initializing Product Recommendation Service ===")

	// Get configuration from environment variables
	config := Config{
		QdrantHost:       getEnvOrDefault("QDRANT_HOST", "localhost"),
		QdrantPort:       getEnvIntOrDefault("QDRANT_PORT", 6333),
		QdrantAPIKey:     os.Getenv("QDRANT_API_KEY"),
		QdrantUseTLS:     getEnvOrDefault("QDRANT_USE_TLS", "false") == "true",
		QdrantCollection: getEnvOrDefault("QDRANT_COLLECTION", "cannabis_products_staging"),
	}

	log.Printf("Product Service Config - Host: %s, Port: %d, Collection: %s, TLS: %t",
		config.QdrantHost, config.QdrantPort, config.QdrantCollection, config.QdrantUseTLS)

	// Create Qdrant client
	qdrantClient, err := qdrant.NewClient(qdrant.Config{
		Host:       config.QdrantHost,
		Port:       config.QdrantPort,
		APIKey:     config.QdrantAPIKey,
		UseTLS:     config.QdrantUseTLS,
		Collection: config.QdrantCollection,
	})
	if err != nil {
		log.Printf("ERROR: Failed to create Qdrant client for product service: %v", err)
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	log.Printf("Product Recommendation Service initialized successfully with collection: %s", config.QdrantCollection)

	return &ProductRecommendationService{
		qdrantClient: qdrantClient,
		openaiClient: openaiClient,
		db:           database,
		collection:   config.QdrantCollection,
	}, nil
}

// GetRecommendations analyzes chat messages and returns product recommendations if appropriate
func (prs *ProductRecommendationService) GetRecommendations(ctx context.Context, messages []chat.ChatMessage) (*ProductRecommendationResponse, error) {
	return prs.GetRecommendationsWithLocation(ctx, messages, nil, false)
}

// GetRecommendationsWithLocation analyzes chat messages and returns location-filtered product recommendations
func (prs *ProductRecommendationService) GetRecommendationsWithLocation(ctx context.Context, messages []chat.ChatMessage, userLocation *UserLocation, isGuest bool) (*ProductRecommendationResponse, error) {
	log.Printf("=== Product Recommendation Analysis Started ===")

	// Minimal decision context: include location note and an explicit rule to proceed on product asks
	decisionMessages := messages
	var preface []chat.ChatMessage
	if userLocation != nil && userLocation.RadiusMile > 0 {
		preface = append(preface, chat.ChatMessage{Role: "system", Content: fmt.Sprintf("Note: User location is already set (lat=%.6f, lng=%.6f, radius=%.1f miles). Do not ask to confirm location.", userLocation.Latitude, userLocation.Longitude, userLocation.RadiusMile)})
	}
	preface = append(preface, chat.ChatMessage{Role: "system", Content: "If the user explicitly asks to see products or for recommendations, set should_show_recommendations=true and provide a specific non-empty recommendation_query that preserves any mentioned product type."})
	if len(preface) > 0 {
		decisionMessages = append(preface, messages...)
	}

	decision, err := prs.openaiClient.ShouldShowProductRecommendations(ctx, decisionMessages)
	if err != nil {
		log.Printf("ERROR: Failed to get recommendation decision: %v", err)
		return nil, fmt.Errorf("failed to get recommendation decision: %w", err)
	}

	log.Printf("Decision: should_show=%t", decision.ShouldShowRecommendations)

	response := &ProductRecommendationResponse{
		ShouldShowRecommendations: decision.ShouldShowRecommendations,
		Products:                  []ProductInfo{},
		Query:                     decision.RecommendationQuery,
		Reason:                    decision.Reason,
	}

	if !decision.ShouldShowRecommendations {
		log.Printf("Not showing recommendations - returning empty response")
		return response, nil
	}

	// Check for location early - if AI says show products but no location, ask for it
	if userLocation == nil || userLocation.RadiusMile <= 0 {
		log.Printf("AI wants to show products but no location provided - requesting location (isGuest: %v)", isGuest)
		response.ShouldShowRecommendations = true
		response.Products = []ProductInfo{}

		// Customize location request message based on user type
		if isGuest {
			response.Reason = fmt.Sprintf("To show you the best %s available near you, I need to know your location first. Please use the location button or settings to share your location and set your search radius.", decision.RecommendationQuery)
		} else {
			response.Reason = fmt.Sprintf("To find the best %s for you, I need to know your location first. Please set your location and search radius in your session settings so I can show you products available in your area.", decision.RecommendationQuery)
		}
		return response, nil
	}

	log.Printf("=== Proceeding with Product Search ===")
	log.Printf("Using location: lat=%.6f, lng=%.6f, radius=%.1f miles", userLocation.Latitude, userLocation.Longitude, userLocation.RadiusMile)

	// Step 2: Generate embeddings for the recommendation query
	log.Printf("Generating embeddings for recommendation query: '%s'", decision.RecommendationQuery)
	embeddings, err := prs.openaiClient.GenerateEmbeddings(ctx, decision.RecommendationQuery)
	if err != nil {
		log.Printf("ERROR: Failed to generate embeddings for query '%s': %v", decision.RecommendationQuery, err)
		log.Printf("No product recommendations to send, at least some acknowledgement that no products exist message should be given")

		// Return a response with explanation when embeddings generation fails
		response.ShouldShowRecommendations = true
		response.Products = []ProductInfo{}
		response.Reason = fmt.Sprintf("I'm having trouble processing your request for %s due to a technical issue with our search system. Please try again in a moment or contact support if this continues.", decision.RecommendationQuery)
		return response, nil
	}
	log.Printf("Successfully generated embeddings (vector size: %d)", len(embeddings))

	// Step 3: Search for similar products in Qdrant
	// Query more products to account for spatial filtering reducing results
	log.Printf("Searching Qdrant collection '%s' with threshold 0.7, limit %d...", prs.collection, constants.QdrantSearchSize)
	searchResults, err := prs.qdrantClient.SearchProducts(ctx, prs.collection, embeddings, constants.QdrantSearchSize, 0.7)
	if err != nil {
		log.Printf("ERROR: Failed to search products in Qdrant collection '%s': %v", prs.collection, err)
		log.Printf("No product recommendations to send, at least some acknowledgement that no products exist message should be given")

		// Return a response with explanation when Qdrant search fails
		response.ShouldShowRecommendations = true
		response.Products = []ProductInfo{}
		response.Reason = fmt.Sprintf("I'm having trouble searching for %s due to a technical issue with our product search system. Please try again in a moment or contact support if this continues.", decision.RecommendationQuery)
		return response, nil
	}
	log.Printf("Qdrant search completed: %d product results found", len(searchResults))

	if len(searchResults) == 0 {
		log.Printf("No products found in Qdrant for query: '%s'", decision.RecommendationQuery)
		log.Printf("No product recommendations to send, at least some acknowledgement that no products exist message should be given")

		// Return a response with explanation when no products are found
		response.ShouldShowRecommendations = true
		response.Products = []ProductInfo{}
		response.Reason = fmt.Sprintf("I understand you're looking for %s, but I don't have any products in my database right now. The product catalog might be empty or still being populated. You might want to check back later or contact support if this issue persists.", decision.RecommendationQuery)
		return response, nil
	}

	// Extract product IDs from Qdrant metadata
	log.Printf("=== Extracting Product IDs from Qdrant Results ===")
	var productIDs []uint64
	for i, result := range searchResults {
		log.Printf("Processing Qdrant result %d: score=%.3f", i+1, result.Score)
		log.Printf("Product Metadata keys: %v", func() []string {
			keys := make([]string, 0, len(result.Product.Metadata))
			for k := range result.Product.Metadata {
				keys = append(keys, k)
			}
			return keys
		}())
		log.Printf("Full metadata for result %d: %+v", i+1, result.Product.Metadata)

		if productIDVal, exists := result.Product.Metadata["product_id"]; exists {
			log.Printf("Result %d: Found product_id in metadata: %v (type: %T)", i+1, productIDVal, productIDVal)

			var productIDStr string
			switch v := productIDVal.(type) {
			case string:
				productIDStr = v
			case float64:
				productIDStr = fmt.Sprintf("%.0f", v)
			case int64:
				productIDStr = fmt.Sprintf("%d", v)
			default:
				log.Printf("Result %d: Unexpected product_id type: %T, value: %v", i+1, v, v)
				continue
			}

			log.Printf("Result %d: Extracted product_id string: '%s'", i+1, productIDStr)

			if productID, err := strconv.ParseUint(productIDStr, 10, 64); err == nil {
				productIDs = append(productIDs, productID)
				log.Printf("Result %d: Successfully parsed product_id: %d", i+1, productID)
			} else {
				log.Printf("Result %d: Failed to parse product_id '%s': %v", i+1, productIDStr, err)
			}
		} else {
			log.Printf("Result %d: product_id not found in metadata", i+1)
		}
	}

	log.Printf("=== Product ID Extraction Summary ===")
	log.Printf("Total unique product IDs extracted: %d", len(productIDs))
	log.Printf("Product IDs: %v", productIDs)
	log.Printf("Converted to NullInt64: %v", prs.convertToNullInt64Slice(productIDs))

	if len(productIDs) == 0 {
		log.Printf("ERROR: No valid product IDs found in Qdrant results")
		log.Printf("No product recommendations to send, at least some acknowledgement that no products exist message should be given")

		// Return a response with explanation when no valid product IDs are found
		response.ShouldShowRecommendations = true
		response.Products = []ProductInfo{}
		response.Reason = fmt.Sprintf("I found some potential matches for %s, but there's a data issue with the product identifiers. The product catalog might have formatting issues. Please try again later or contact support.", decision.RecommendationQuery)
		return response, nil
	}

	// Fetch all product listings from database using the product IDs
	log.Printf("=== Fetching Product Listings from Database ===")
	log.Printf("Total product IDs from Qdrant: %d", len(productIDs))

	queries := db.New(prs.db)

	// At this point we are guaranteed to have valid location data
	// Limit radius to prevent performance issues
	effectiveRadius := userLocation.RadiusMile
	if effectiveRadius > 62 {
		log.Printf("Limiting search radius from %.1f miles to 62 miles for performance", effectiveRadius)
		effectiveRadius = 62
	}

	// Progressive querying: Query in batches (15:20:25 ratio) to optimize performance
	allListings, err := prs.queryProductsInBatches(ctx, queries, productIDs, userLocation, effectiveRadius)
	if err != nil {
		log.Printf("ERROR: Failed to fetch location-filtered product listings: %v", err)
		response.ShouldShowRecommendations = true
		response.Products = []ProductInfo{}
		response.Reason = "I'm having trouble finding products near your location. Please try again in a moment."
		return response, nil
	}

	log.Printf("=== Deduplicating Products ===")
	log.Printf("Total listings before deduplication: %d", len(allListings))

	// Log product ID distribution
	productIDCounts := make(map[int64]int)
	for _, product := range allListings {
		if product.ProductID != nil {
			productIDCounts[*product.ProductID]++
		}
	}
	log.Printf("Product listings by product_id: %v", productIDCounts)

	// Deduplicate and limit to MaxProductsToShow
	allProducts := prs.deduplicateAndLimitProducts(allListings, userLocation, constants.MaxProductsToShow)

	// Enrich products with AI-generated insights
	log.Printf("=== Enriching Products with AI Insights ===")
	allProducts = prs.enrichProductsWithAIInsights(ctx, queries, allProducts)

	log.Printf("=== SQL Query Results Summary ===")
	log.Printf("Total unique products after deduplication: %d", len(allProducts))

	// Check if we have any products after processing
	if len(allProducts) == 0 {
		log.Printf("No products within user's radius (%.1f miles), attempting fallback search", effectiveRadius)
		return prs.handleFallbackSearch(ctx, queries, productIDs, userLocation, effectiveRadius, decision.RecommendationQuery, response)
	}

	// Products found within user's radius - set products and success message
	response.Products = allProducts

	// Generate a contextual message for products found within radius
	response.Reason = constants.GenerateSuccessMessage(
		len(allProducts),
		decision.RecommendationQuery,
		effectiveRadius,
	)
	log.Printf("Set success message: '%s'", response.Reason)

	log.Printf("=== Final Product Recommendation Response ===")
	log.Printf("Should show recommendations: %t", response.ShouldShowRecommendations)
	log.Printf("Number of products: %d", len(response.Products))
	log.Printf("Query: '%s'", response.Query)
	log.Printf("Reason: '%s'", response.Reason)

	// Log each product in the final response
	for i, product := range response.Products {
		thcContent := 0.0
		if product.THCContent != nil {
			thcContent = *product.THCContent
		}
		cbdContent := 0.0
		if product.CBDContent != nil {
			cbdContent = *product.CBDContent
		}
		log.Printf("Final Product %d: ID=%s, Name='%s', Price=%.2f, Category='%s', THC=%.1f%%, CBD=%.1f%%",
			i+1, product.ID, product.Name, product.Price, product.Category, thcContent, cbdContent)
	}

	return response, nil
}

// Helper function to extract product name from description or metadata
func (prs *ProductRecommendationService) extractNameFromDescription(description sql.NullString) string {
	if !description.Valid {
		return "Cannabis Product"
	}

	desc := description.String
	if len(desc) > 100 {
		// Try to extract the first line or first sentence as the name
		if idx := strings.Index(desc, "\n"); idx > 0 && idx < 100 {
			return strings.TrimSpace(desc[:idx])
		}
		if idx := strings.Index(desc, "."); idx > 0 && idx < 100 {
			return strings.TrimSpace(desc[:idx])
		}
		// Just take first 100 characters
		return strings.TrimSpace(desc[:100]) + "..."
	}

	return strings.TrimSpace(desc)
}

// Helper function to safely extract string values
func (prs *ProductRecommendationService) getStringValue(nullString sql.NullString) string {
	if nullString.Valid {
		return nullString.String
	}
	return ""
}

// Helper function to safely extract float values
func (prs *ProductRecommendationService) getFloatValue(nullFloat sql.NullFloat64) float64 {
	if nullFloat.Valid {
		return nullFloat.Float64
	}
	return 0.0
}

// Helper function to safely extract float pointers
func (prs *ProductRecommendationService) getFloatPointer(nullFloat sql.NullFloat64) *float64 {
	if nullFloat.Valid {
		f := nullFloat.Float64
		return &f
	}
	return nil
}

// Helper function to safely extract int values
func (prs *ProductRecommendationService) getIntValue(nullInt sql.NullInt64) int32 {
	if nullInt.Valid {
		return int32(nullInt.Int64)
	}
	return 0
}

// Helper function to safely extract int32 values
func (prs *ProductRecommendationService) getInt32Value(nullInt sql.NullInt32) int32 {
	if nullInt.Valid {
		return nullInt.Int32
	}
	return 0
}

// Helper function to safely extract int pointers
func (prs *ProductRecommendationService) getIntPointer(nullInt sql.NullInt64) *int64 {
	if nullInt.Valid {
		i := nullInt.Int64
		return &i
	}
	return nil
}

// Helper function to convert NullString to float64 (for price fields stored as strings)
func (prs *ProductRecommendationService) getStringAsFloat(nullString sql.NullString) float64 {
	if nullString.Valid {
		if f, err := strconv.ParseFloat(nullString.String, 64); err == nil {
			return f
		}
	}
	return 0.0
}

// Helper function to convert NullString to *float64 (for nullable price fields stored as strings)
func (prs *ProductRecommendationService) getStringAsFloatPointer(nullString sql.NullString) *float64 {
	if nullString.Valid {
		if f, err := strconv.ParseFloat(nullString.String, 64); err == nil {
			return &f
		}
	}
	return nil
}

// Helper function to process and validate image URLs
func (prs *ProductRecommendationService) processImageURLs(imageURLs sql.NullString) string {
	if !imageURLs.Valid || strings.TrimSpace(imageURLs.String) == "" {
		// Return a default placeholder image for cannabis products
		log.Printf("No image URL available, using default placeholder")
		return "https://via.placeholder.com/300x200/2d5a3d/ffffff?text=Cannabis+Product"
	}

	// Clean and validate the image URL
	imageURL := strings.TrimSpace(imageURLs.String)

	// Handle JSON array format (e.g., ["url1", "url2"])
	if strings.HasPrefix(imageURL, "[") && strings.HasSuffix(imageURL, "]") {
		var urls []string
		if err := json.Unmarshal([]byte(imageURL), &urls); err == nil && len(urls) > 0 {
			imageURL = strings.TrimSpace(urls[0])
		} else {
			return "https://via.placeholder.com/300x200/2d5a3d/ffffff?text=Cannabis+Product"
		}
	}

	/*if strings.Contains(imageURL, ",") {
		parts := strings.Split(imageURL, ",")
		imageURL = strings.TrimSpace(parts[0])
		log.Printf("Multiple comma-separated URLs found, using first: %s", imageURL)
	} else if strings.Contains(imageURL, ";") {
		parts := strings.Split(imageURL, ";")
		imageURL = strings.TrimSpace(parts[0])
		log.Printf("Multiple semicolon-separated URLs found, using first: %s", imageURL)
	}
	*/
	return imageURL
}

// Helper functions
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// Helper function to convert float64 to *float64 pointer (only if non-zero)
func (prs *ProductRecommendationService) getFloatPointerFromValue(value float64) *float64 {
	if value != 0 {
		return &value
	}
	return nil
}

// Convert uint64 slice to sql.NullInt64 slice for database queries
func (prs *ProductRecommendationService) convertToNullInt64Slice(productIDs []uint64) []sql.NullInt64 {
	nullInt64s := make([]sql.NullInt64, len(productIDs))
	for i, id := range productIDs {
		nullInt64s[i] = sql.NullInt64{Int64: int64(id), Valid: true}
	}
	return nullInt64s
}

// Helper function to determine if a product should replace an existing one (with location preference)
func (prs *ProductRecommendationService) shouldReplaceProductWithLocation(newProduct, existingProduct ProductInfo, userLocation *UserLocation) bool {
	// If we have location data, prioritize by distance first
	if userLocation != nil && newProduct.DistanceMile != nil && existingProduct.DistanceMile != nil {
		distanceDiff := *newProduct.DistanceMile - *existingProduct.DistanceMile

		// If new product is significantly closer (more than 3 miles), prefer it
		if distanceDiff < -3.0 {
			return true
		}

		// If new product is significantly farther (more than 3 miles), don't replace
		if distanceDiff > 3.0 {
			return false
		}

		// If distances are similar (within 3 miles), use price/inventory logic
	}

	// Default logic: prefer lower price, then higher inventory
	if newProduct.Price < existingProduct.Price {
		return true
	} else if newProduct.Price == existingProduct.Price && newProduct.InventoryCount > existingProduct.InventoryCount {
		return true
	}

	return false
}

// convertRowsToProductInfo converts SQL rows to ProductInfo without deduplication
// Products without a valid product_name are skipped
func (prs *ProductRecommendationService) convertRowsToProductInfo(spatialRows []db.GetProductListingsWithinRadiusByProductIDsRow) []ProductInfo {
	var products []ProductInfo
	skippedCount := 0

	for _, row := range spatialRows {
		// Skip products without a valid product_name
		if !row.ProductName.Valid || strings.TrimSpace(row.ProductName.String) == "" {
			skippedCount++
			log.Printf("Skipping product listing %d: no valid product_name", row.ListingID)
			continue
		}

		productName := strings.TrimSpace(row.ProductName.String)

		product := ProductInfo{
			ID:              fmt.Sprintf("%d", row.ListingID),
			Name:            productName,
			Description:     prs.getStringValue(row.Description),
			Price:           prs.getFloatValue(row.Price),
			ImageURL:        prs.processImageURLs(row.ImageUrls),
			Category:        "",
			THCContent:      prs.getStringAsFloatPointer(row.ThcPercentage),
			CBDContent:      prs.getStringAsFloatPointer(row.CbdPercentage),
			ListingURL:      prs.getStringValue(row.ListingUrl),
			RetailerID:      prs.getIntPointer(row.RetailerID),
			ProductID:       prs.getIntPointer(row.ProductID),
			InventoryCount:  prs.getInt32Value(row.InventoryCount),
			DiscountedPrice: prs.getFloatPointer(row.DiscountedPrice),
			// Location fields
			RetailerName:    prs.getStringValue(row.RetailerName),
			RetailerLat:     prs.getFloatPointer(row.RetailerLatitude),
			RetailerLng:     prs.getFloatPointer(row.RetailerLongitude),
			DistanceMile:    prs.getFloatPointerFromValue(row.DistanceMile),
			RetailerHomeURL: prs.getStringValue(row.RetailerHomeURL),
			RetailerMenuURL: prs.getStringValue(row.RetailerMenuURL),
		}
		products = append(products, product)
	}

	if skippedCount > 0 {
		log.Printf("Skipped %d products without valid product_name", skippedCount)
	}

	return products
}

// deduplicateProducts deduplicates products by product_id, keeping the best listing for each product
func (prs *ProductRecommendationService) deduplicateProducts(allListings []ProductInfo, userLocation *UserLocation) []ProductInfo {
	productMap := make(map[int64]ProductInfo)
	var finalProducts []ProductInfo

	for _, product := range allListings {
		// Deduplicate by product_id
		if product.ProductID != nil {
			productID := *product.ProductID

			if existingProduct, exists := productMap[productID]; exists {
				shouldReplace := prs.shouldReplaceProductWithLocation(product, existingProduct, userLocation)
				if shouldReplace {
					productMap[productID] = product
				}
			} else {
				// First time seeing this product_id
				productMap[productID] = product
			}
		} else {
			// If no product_id, add it directly (shouldn't happen with proper data)
			finalProducts = append(finalProducts, product)
		}
	}

	// Convert map back to slice
	for _, product := range productMap {
		finalProducts = append(finalProducts, product)
	}

	log.Printf("Deduplication complete: %d listings → %d unique products", len(allListings), len(finalProducts))
	return finalProducts
}

// deduplicateAndLimitProducts deduplicates products and limits to maxProducts
func (prs *ProductRecommendationService) deduplicateAndLimitProducts(allListings []ProductInfo, userLocation *UserLocation, maxProducts int) []ProductInfo {
	// First deduplicate
	deduplicated := prs.deduplicateProducts(allListings, userLocation)

	// Then limit to maxProducts
	if len(deduplicated) > maxProducts {
		log.Printf("Limiting products from %d to %d (MaxProductsToShow)", len(deduplicated), maxProducts)
		// Sort by distance if available, then take the closest ones
		deduplicated = prs.selectTopProducts(deduplicated, userLocation, maxProducts)
	}

	return deduplicated
}

// selectTopProducts selects the top N products based on distance and quality criteria
func (prs *ProductRecommendationService) selectTopProducts(products []ProductInfo, userLocation *UserLocation, maxProducts int) []ProductInfo {
	if len(products) <= maxProducts {
		return products
	}

	// Sort products by distance (closest first), then by price (lowest first)
	// Using a simple selection approach - prioritize by distance
	type scoredProduct struct {
		product  ProductInfo
		distance float64
		price    float64
	}

	scored := make([]scoredProduct, len(products))
	for i, p := range products {
		distance := float64(999999) // Default to very far if no distance
		if p.DistanceMile != nil {
			distance = *p.DistanceMile
		}
		scored[i] = scoredProduct{
			product:  p,
			distance: distance,
			price:    p.Price,
		}
	}

	// Sort by distance first, then by price
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			shouldSwap := false
			if scored[i].distance > scored[j].distance {
				shouldSwap = true
			} else if scored[i].distance == scored[j].distance && scored[i].price > scored[j].price {
				shouldSwap = true
			}
			if shouldSwap {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Take top maxProducts
	result := make([]ProductInfo, maxProducts)
	for i := 0; i < maxProducts; i++ {
		result[i] = scored[i].product
	}

	log.Printf("Selected top %d products by distance and price", maxProducts)
	return result
}

// queryProductsInBatches queries products in progressive batches (15:20:25 ratio)
// Returns early if MinProductsToShow unique products are found
func (prs *ProductRecommendationService) queryProductsInBatches(
	ctx context.Context,
	queries *db.Queries,
	productIDs []uint64,
	userLocation *UserLocation,
	effectiveRadius float64,
) ([]ProductInfo, error) {
	batchRanges := constants.GetBatchRanges(len(productIDs))
	log.Printf("=== Progressive Batch Querying ===")
	log.Printf("Total product IDs: %d, Number of batches: %d", len(productIDs), len(batchRanges))
	log.Printf("Batch ranges: %v", batchRanges)

	var allListings []ProductInfo
	uniqueProductIDs := make(map[int64]bool)

	for batchNum, batchRange := range batchRanges {
		batchStart := batchRange[0]
		batchEnd := batchRange[1]
		batchProductIDs := productIDs[batchStart:batchEnd]

		log.Printf("=== Progressive Query: Batch %d ===", batchNum+1)
		log.Printf("Querying batch %d of %d product IDs (indices %d-%d): %v",
			batchNum+1, len(batchProductIDs), batchStart, batchEnd-1, batchProductIDs)

		// Query this batch
		spatialRows, err := queries.GetProductListingsWithinRadiusByProductIDs(ctx, db.GetProductListingsWithinRadiusByProductIDsParams{
			UserLat:    userLocation.Latitude,
			UserLng:    userLocation.Longitude,
			ProductIds: prs.convertToNullInt64Slice(batchProductIDs),
			RadiusMile: effectiveRadius,
		})

		if err != nil {
			log.Printf("ERROR: Failed to fetch batch %d: %v", batchNum+1, err)
			// If first batch fails, return error; otherwise continue with what we have
			if batchNum == 0 {
				return nil, err
			}
			log.Printf("Continuing with previous batch results despite batch %d error", batchNum+1)
			break
		}

		log.Printf("Batch %d returned %d product listings", batchNum+1, len(spatialRows))

		// Convert batch to ProductInfo
		batchListings := prs.convertRowsToProductInfo(spatialRows)
		log.Printf("Batch %d has %d valid listings after filtering", batchNum+1, len(batchListings))

		// Count unique products in this batch
		for _, product := range batchListings {
			if product.ProductID != nil {
				uniqueProductIDs[*product.ProductID] = true
			}
		}

		// Add batch listings to all listings
		allListings = append(allListings, batchListings...)

		uniqueCount := len(uniqueProductIDs)
		log.Printf("After batch %d: %d unique product IDs (from %d total listings)",
			batchNum+1, uniqueCount, len(allListings))

		// Check if we have enough unique products to stop querying
		if uniqueCount >= constants.MinProductsToShow {
			remainingBatches := len(batchRanges) - batchNum - 1
			if remainingBatches > 0 {
				log.Printf("Found %d unique products (>= %d), skipping remaining %d batch(es)",
					uniqueCount, constants.MinProductsToShow, remainingBatches)
			}
			break
		}

		// If more batches available and we don't have enough products, continue
		if batchNum < len(batchRanges)-1 {
			log.Printf("Only %d unique products (< %d), will query next batch",
				uniqueCount, constants.MinProductsToShow)
		}
	}

	log.Printf("=== Batch Querying Complete ===")
	log.Printf("Total listings collected: %d, Unique product IDs: %d", len(allListings), len(uniqueProductIDs))

	return allListings, nil
}

// handleFallbackSearch performs progressive fallback searches when no products found in user's radius
func (prs *ProductRecommendationService) handleFallbackSearch(
	ctx context.Context,
	queries *db.Queries,
	productIDs []uint64,
	userLocation *UserLocation,
	originalRadius float64,
	productQuery string,
	response *ProductRecommendationResponse,
) (*ProductRecommendationResponse, error) {
	// Progressive fallback: try expanding radius to find products
	fallbackTiers := constants.GetFallbackTiers()
	var fallbackProducts []ProductInfo
	var usedFallbackRadius float64

	for _, fallbackRadius := range fallbackTiers {
		// Skip if user's radius already exceeds this tier
		if fallbackRadius <= originalRadius {
			log.Printf("Skipping fallback tier %.0f miles (user radius %.1f miles already exceeds it)", fallbackRadius, originalRadius)
			continue
		}

		log.Printf("Attempting fallback search with %.0f-mile radius", fallbackRadius)

		// Query with expanded radius using existing query
		fallbackRows, err := queries.GetProductListingsWithinRadiusByProductIDs(ctx, db.GetProductListingsWithinRadiusByProductIDsParams{
			UserLat:    userLocation.Latitude,
			UserLng:    userLocation.Longitude,
			ProductIds: prs.convertToNullInt64Slice(productIDs),
			RadiusMile: fallbackRadius,
		})

		if err != nil {
			log.Printf("ERROR: Fallback query at %.0f miles failed: %v", fallbackRadius, err)
			continue // Try next tier
		}

		if len(fallbackRows) > 0 {
			log.Printf("Fallback query at %.0f miles returned %d listings", fallbackRadius, len(fallbackRows))

			// Convert, deduplicate, and limit to MaxProductsToShow
			fallbackListings := prs.convertRowsToProductInfo(fallbackRows)
			fallbackProducts = prs.deduplicateAndLimitProducts(fallbackListings, userLocation, constants.MaxProductsToShow)

			// Enrich fallback products with AI insights
			if len(fallbackProducts) > 0 {
				log.Printf("Enriching fallback products with AI insights")
				fallbackProducts = prs.enrichProductsWithAIInsights(ctx, queries, fallbackProducts)
				usedFallbackRadius = fallbackRadius
				log.Printf("✅ Fallback successful: found %d unique products at %.0f miles", len(fallbackProducts), fallbackRadius)
				break // Found products, stop searching
			}
		}

		log.Printf("Fallback at %.0f miles returned no products, trying next tier", fallbackRadius)
	}

	// If we found products in fallback, return them with contextual message
	if len(fallbackProducts) > 0 {
		return prs.buildFallbackSuccessResponse(
			fallbackProducts,
			productQuery,
			originalRadius,
			usedFallbackRadius,
			response,
		), nil
	}

	// Ultimate fallback: nothing found even at maximum fallback radius
	log.Printf("No products found even after fallback searches up to %.0f miles", fallbackTiers[len(fallbackTiers)-1])
	response.ShouldShowRecommendations = true
	response.Products = []ProductInfo{}
	response.Reason = constants.GenerateNoProductsMessage(productQuery)
	return response, nil
}

// buildFallbackSuccessResponse constructs the response when fallback search finds products
func (prs *ProductRecommendationService) buildFallbackSuccessResponse(
	products []ProductInfo,
	productQuery string,
	originalRadius float64,
	fallbackRadius float64,
	response *ProductRecommendationResponse,
) *ProductRecommendationResponse {
	response.ShouldShowRecommendations = true
	response.Products = products
	response.Reason = constants.GenerateFallbackFoundMessage(
		productQuery,
		originalRadius,
		len(products),
		fallbackRadius,
	)

	log.Printf("Returning %d fallback products with expanded radius message", len(products))
	return response
}

// enrichProductsWithAIInsights enriches products with AI-generated insights
// It first checks the database cache, then generates missing insights via OpenAI
func (prs *ProductRecommendationService) enrichProductsWithAIInsights(ctx context.Context, queries *db.Queries, products []ProductInfo) []ProductInfo {
	if len(products) == 0 {
		return products
	}

	// Collect all product IDs
	var productIDs []int64
	productIDMap := make(map[int64]int) // productID -> index in products slice
	for i, p := range products {
		if p.ProductID != nil {
			productIDs = append(productIDs, *p.ProductID)
			productIDMap[*p.ProductID] = i
		}
	}

	if len(productIDs) == 0 {
		log.Printf("No product IDs found for AI insight enrichment")
		return products
	}

	log.Printf("Looking up cached AI insights for %d products", len(productIDs))

	// Step 1: Check database for existing insights
	cachedInsights, err := queries.GetProductAIInsightsByProductIDs(ctx, productIDs)
	if err != nil {
		log.Printf("WARNING: Failed to fetch cached AI insights: %v", err)
		// Continue without cached insights
		cachedInsights = []db.ProductAIInsight{}
	}

	log.Printf("Found %d cached AI insights", len(cachedInsights))

	// Build a map of cached insights by product ID
	cachedMap := make(map[int64]db.ProductAIInsight)
	for _, insight := range cachedInsights {
		cachedMap[int64(insight.ProductID)] = insight
	}

	// Identify products that need AI generation
	var productsNeedingAI []chat.ProductInsightsInput
	for _, p := range products {
		if p.ProductID == nil {
			continue
		}
		productID := *p.ProductID
		if _, exists := cachedMap[productID]; !exists {
			// Build input for AI generation
			thcContent := 0.0
			cbdContent := 0.0
			if p.THCContent != nil {
				thcContent = *p.THCContent
			}
			if p.CBDContent != nil {
				cbdContent = *p.CBDContent
			}

			productsNeedingAI = append(productsNeedingAI, chat.ProductInsightsInput{
				ProductID:   productID,
				Name:        p.Name,
				Description: p.Description,
				Category:    p.Category,
				THCContent:  thcContent,
				CBDContent:  cbdContent,
				StrainType:  p.StrainType,
			})
		}
	}

	log.Printf("%d products need AI insight generation", len(productsNeedingAI))

	// Step 2: Generate insights for products not in cache
	if len(productsNeedingAI) > 0 {
		aiInsights, err := prs.openaiClient.GenerateProductInsights(ctx, productsNeedingAI)
		if err != nil {
			log.Printf("WARNING: Failed to generate AI insights: %v", err)
			// Continue with cached insights only
		} else {
			log.Printf("Generated AI insights for %d products", len(aiInsights))

			// Save generated insights to database
			for _, insight := range aiInsights {
				if err := prs.saveProductInsight(ctx, queries, insight); err != nil {
					log.Printf("WARNING: Failed to save AI insight for product %d: %v", insight.ProductID, err)
				}
			}

			// Add to cached map for enrichment
			for _, insight := range aiInsights {
				// Convert to database format for consistency
				topFeelingsJSON, _ := json.Marshal(insight.TopFeelings)
				keyInfoJSON, _ := json.Marshal(insight.KeyInfo)

				cachedMap[insight.ProductID] = db.ProductAIInsight{
					ProductID:      uint64(insight.ProductID),
					WhyThisProduct: sql.NullString{String: insight.WhyThisProduct, Valid: insight.WhyThisProduct != ""},
					TopFeelings:    topFeelingsJSON,
					KeyInfo:        keyInfoJSON,
					Ingredients:    sql.NullString{String: insight.Ingredients, Valid: insight.Ingredients != ""},
				}
			}
		}
	}

	// Step 3: Enrich products with insights from cache
	for i, p := range products {
		if p.ProductID == nil {
			continue
		}
		productID := *p.ProductID

		if insight, exists := cachedMap[productID]; exists {
			products[i] = prs.applyInsightToProduct(p, insight)
		}
	}

	log.Printf("Enriched %d products with AI insights", len(products))
	return products
}

// saveProductInsight saves a product insight to the database
func (prs *ProductRecommendationService) saveProductInsight(ctx context.Context, queries *db.Queries, insight chat.ProductInsightsOutput) error {
	topFeelingsJSON, err := json.Marshal(insight.TopFeelings)
	if err != nil {
		return fmt.Errorf("failed to marshal top_feelings: %w", err)
	}

	keyInfoJSON, err := json.Marshal(insight.KeyInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal key_info: %w", err)
	}

	return queries.UpsertProductAIInsight(ctx, db.UpsertProductAIInsightParams{
		ProductID:      insight.ProductID,
		WhyThisProduct: sql.NullString{String: insight.WhyThisProduct, Valid: insight.WhyThisProduct != ""},
		TopFeelings:    topFeelingsJSON,
		KeyInfo:        keyInfoJSON,
		Ingredients:    sql.NullString{String: insight.Ingredients, Valid: insight.Ingredients != ""},
	})
}

// applyInsightToProduct applies cached AI insight to a product
func (prs *ProductRecommendationService) applyInsightToProduct(product ProductInfo, insight db.ProductAIInsight) ProductInfo {
	// Apply "Why This Product"
	if insight.WhyThisProduct.Valid {
		product.WhyThisProduct = insight.WhyThisProduct.String
	}

	// Apply "Top Feelings"
	if len(insight.TopFeelings) > 0 {
		var feelings []string
		if err := json.Unmarshal(insight.TopFeelings, &feelings); err == nil {
			product.TopFeelings = feelings
		}
	}

	// Apply "Key Info"
	if len(insight.KeyInfo) > 0 {
		var keyInfo KeyInfo
		if err := json.Unmarshal(insight.KeyInfo, &keyInfo); err == nil {
			product.KeyInfo = &keyInfo
		}
	}

	// Apply "Ingredients"
	if insight.Ingredients.Valid {
		product.Ingredients = insight.Ingredients.String
	}

	return product
}

// Close closes the Qdrant client connection
func (prs *ProductRecommendationService) Close() error {
	return prs.qdrantClient.Close()
}
