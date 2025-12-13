package chat

import (
	"cannabee/pkg/qdrant"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// KnowledgeService handles retrieval of cannabis knowledge from Qdrant
type KnowledgeService struct {
	qdrantClient        *qdrant.Client
	openaiClient        *OpenAIClient
	knowledgeCollection string
}

// KnowledgeConfig holds configuration for the knowledge service
type KnowledgeConfig struct {
	QdrantHost          string
	QdrantPort          int
	QdrantAPIKey        string
	QdrantUseTLS        bool
	KnowledgeCollection string
}

// KnowledgeResult represents a piece of cannabis knowledge retrieved from Qdrant
type KnowledgeResult struct {
	Content  string                 `json:"content"`
	Source   string                 `json:"source,omitempty"`
	Score    float64                `json:"score"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// KnowledgeResponse represents the response containing relevant cannabis knowledge
type KnowledgeResponse struct {
	Query                string            `json:"query"`
	Results              []KnowledgeResult `json:"results"`
	ContextText          string            `json:"context_text"`
	HasRelevantKnowledge bool              `json:"has_relevant_knowledge"`
}

// NewKnowledgeService creates a new cannabis knowledge service
func NewKnowledgeService(openaiClient *OpenAIClient) (*KnowledgeService, error) {
	log.Printf("Initializing Cannabis Knowledge Service...")

	// Get configuration from environment variables
	config := KnowledgeConfig{
		QdrantHost:          getEnvOrDefault("QDRANT_HOST", "localhost"),
		QdrantPort:          getEnvIntOrDefault("QDRANT_PORT", 6333),
		QdrantAPIKey:        os.Getenv("QDRANT_API_KEY"),
		QdrantUseTLS:        getEnvOrDefault("QDRANT_USE_TLS", "false") == "true",
		KnowledgeCollection: getEnvOrDefault("QDRANT_KNOWLEDGE_COLLECTION", "cannabis_knowledge_documents"),
	}

	log.Printf("Knowledge Service Config - Host: %s, Port: %d, Collection: %s, TLS: %t",
		config.QdrantHost, config.QdrantPort, config.KnowledgeCollection, config.QdrantUseTLS)

	// Create Qdrant client
	qdrantClient, err := qdrant.NewClient(qdrant.Config{
		Host:       config.QdrantHost,
		Port:       config.QdrantPort,
		APIKey:     config.QdrantAPIKey,
		UseTLS:     config.QdrantUseTLS,
		Collection: config.KnowledgeCollection,
	})
	if err != nil {
		log.Printf("Failed to create Qdrant client for knowledge service: %v", err)
		return nil, fmt.Errorf("failed to create Qdrant client for knowledge service: %w", err)
	}

	log.Printf("Cannabis Knowledge Service initialized successfully with collection: %s", config.KnowledgeCollection)

	return &KnowledgeService{
		qdrantClient:        qdrantClient,
		openaiClient:        openaiClient,
		knowledgeCollection: config.KnowledgeCollection,
	}, nil
}

// GetRelevantKnowledge retrieves relevant cannabis knowledge based on user query
func (ks *KnowledgeService) GetRelevantKnowledge(ctx context.Context, userQuery string) (*KnowledgeResponse, error) {
	log.Printf("=== Cannabis Knowledge Retrieval Started ===")
	log.Printf("User Query: '%s'", userQuery)
	log.Printf("Collection: %s", ks.knowledgeCollection)

	// Generate embeddings for the user query
	log.Printf("Generating embeddings for knowledge query...")
	embeddings, err := ks.openaiClient.GenerateEmbeddings(ctx, userQuery)
	if err != nil {
		log.Printf("ERROR: Failed to generate embeddings for knowledge query: %v", err)
		return nil, fmt.Errorf("failed to generate embeddings for knowledge query: %w", err)
	}
	log.Printf("Successfully generated embeddings (vector size: %d)", len(embeddings))

	log.Printf("Searching cannabis knowledge collection '%s' with similarity threshold 0.6...", ks.knowledgeCollection)

	// Search for relevant knowledge in Qdrant
	searchResults, err := ks.qdrantClient.SearchProducts(ctx, ks.knowledgeCollection, embeddings, 5, 0.6)
	if err != nil {
		log.Printf("ERROR: Failed to search cannabis knowledge in collection '%s': %v", ks.knowledgeCollection, err)
		return &KnowledgeResponse{
			Query:                userQuery,
			Results:              []KnowledgeResult{},
			ContextText:          "",
			HasRelevantKnowledge: false,
		}, nil
	}

	log.Printf("Qdrant search completed: %d knowledge documents found", len(searchResults))

	// Convert search results to knowledge results
	knowledgeResults := make([]KnowledgeResult, 0, len(searchResults))
	var contextParts []string
	highRelevanceCount := 0

	for i, result := range searchResults {
		log.Printf("Processing knowledge result %d: score=%.3f", i+1, result.Score)

		content := result.Product.Description
		if content == "" {
			content = result.Product.Name
			log.Printf("Knowledge result %d: Using name as content (description empty)", i+1)
		}

		// Check if content is still empty and try to extract from metadata "text" field
		if content == "" {
			if textVal, exists := result.Product.Metadata["text"]; exists {
				if textStr, ok := textVal.(string); ok {
					content = textStr
					log.Printf("Knowledge result %d: Using text from metadata (description and name empty)", i+1)
				}
			}
		}
		log.Printf("Knowledge result %d: content_length=%d", i+1, len(content))

		// Extract source information from metadata
		source := ""
		if sourceVal, exists := result.Product.Metadata["source"]; exists {
			if sourceStr, ok := sourceVal.(string); ok {
				source = sourceStr
				log.Printf("Knowledge result %d: source='%s'", i+1, source)
			}
		}

		knowledgeResult := KnowledgeResult{
			Content:  content,
			Source:   source,
			Score:    result.Score,
			Metadata: result.Product.Metadata,
		}

		knowledgeResults = append(knowledgeResults, knowledgeResult)

		// Add to context text if score is high enough
		if result.Score >= 0.7 {
			contextParts = append(contextParts, fmt.Sprintf("Knowledge %d: %s", i+1, content))
			highRelevanceCount++
			log.Printf("Knowledge result %d: INCLUDED in context (high relevance score: %.3f)", i+1, result.Score)
		} else {
			log.Printf("Knowledge result %d: EXCLUDED from context (low relevance score: %.3f)", i+1, result.Score)
		}

		// Log metadata keys for debugging
		if len(result.Product.Metadata) > 0 {
			metadataKeys := make([]string, 0, len(result.Product.Metadata))
			for key := range result.Product.Metadata {
				metadataKeys = append(metadataKeys, key)
			}
			log.Printf("Knowledge result %d: metadata_keys=%v", i+1, metadataKeys)
		}
	}

	// Combine context parts into a single context text
	contextText := strings.Join(contextParts, "\n\n")
	hasRelevantKnowledge := len(contextParts) > 0

	response := &KnowledgeResponse{
		Query:                userQuery,
		Results:              knowledgeResults,
		ContextText:          contextText,
		HasRelevantKnowledge: hasRelevantKnowledge,
	}

	log.Printf("=== Cannabis Knowledge Retrieval Summary ===")
	log.Printf("Total results: %d", len(knowledgeResults))
	log.Printf("High-relevance results (score >= 0.7): %d", highRelevanceCount)
	log.Printf("Context length: %d characters", len(contextText))
	log.Printf("Has relevant knowledge: %t", hasRelevantKnowledge)
	if hasRelevantKnowledge {
		log.Printf("Context preview: %.200s...", contextText)
	} else {
		log.Printf("No high-relevance knowledge found for query: '%s'", userQuery)
	}
	log.Printf("=== Cannabis Knowledge Retrieval Complete ===")

	return response, nil
}

// Helper functions for environment variables
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
