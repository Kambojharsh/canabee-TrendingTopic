package opensearch

import (
	"time"
)

// KnowledgeDocument represents a cannabis knowledge document stored in OpenSearch
type KnowledgeDocument struct {
	// Core identification
	ContentHash string `json:"content_hash"` // SHA256 hash of normalized embedding text
	ID          string `json:"id,omitempty"` // Optional external ID

	// Embedding data
	EmbeddingText   string    `json:"embedding_text"`   // Plain text used for embedding
	EmbeddingVector []float32 `json:"embedding_vector"` // Vector representation

	// Source tracking
	SourceType string `json:"source_type"` // duckduckgo | pubmed | user_generated | brand
	SourceURL  string `json:"source_url,omitempty"`
	RawContent string `json:"raw_content,omitempty"` // Original unprocessed content

	// Classification metadata (from cannabis-knowledge-metadata.json schema)
	ContentType string `json:"content_type,omitempty"` // educational | research | product_info
	Category    string `json:"category,omitempty"`     // flower | edible | vape | concentrate | tincture | topical | etc.
	SubCategory string `json:"sub_category,omitempty"` // gummy | chocolate | cartridge | etc.
	StrainType  string `json:"strain_type,omitempty"`  // indica | sativa | hybrid | cbd | cbg | balanced

	// Cannabinoid profile
	THCLevel string `json:"thc_level,omitempty"` // low | medium | high
	CBDLevel string `json:"cbd_level,omitempty"` // low | medium | high

	// Effects and ailments
	PrimaryAilments   []string `json:"primary_ailments,omitempty"`   // sleep, pain, anxiety, etc.
	PrimaryEffects    []string `json:"primary_effects,omitempty"`    // relaxed, euphoric, focused, etc.
	MedicalBenefits   []string `json:"medical_benefits,omitempty"`   // pain_relief, insomnia, etc.
	ConsumptionMethod []string `json:"consumption_method,omitempty"` // smoke, vape, eat, sublingual, topical

	// User targeting
	ExperienceLevel []string `json:"experience_level,omitempty"` // beginner, intermediate, experienced
	TimeOfUse       string   `json:"time_of_use,omitempty"`      // daytime | evening | anytime

	// Flavor and aroma
	PrimaryFlavors []string `json:"primary_flavors,omitempty"` // citrus, earthy, sweet, etc.

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Optional TTL for content freshness
	TTL *time.Time `json:"ttl,omitempty"`
}

// SearchResult represents a single search result from OpenSearch
type SearchResult struct {
	Document KnowledgeDocument `json:"document"`
	Score    float64           `json:"score"`
	ID       string            `json:"id"`
}

// SearchResponse represents the response from a knowledge search
type SearchResponse struct {
	Results    []SearchResult `json:"results"`
	TotalHits  int64          `json:"total_hits"`
	TookMs     int64          `json:"took_ms"`
	Query      string         `json:"query,omitempty"`
	SourceType string         `json:"source_type,omitempty"` // Filter applied
}

// SearchFilters represents metadata filters for knowledge search
type SearchFilters struct {
	SourceType        string   `json:"source_type,omitempty"`        // duckduckgo | pubmed
	ContentType       string   `json:"content_type,omitempty"`       // educational | research
	Category          string   `json:"category,omitempty"`           // flower | edible | etc.
	StrainType        string   `json:"strain_type,omitempty"`        // indica | sativa | hybrid
	THCLevel          string   `json:"thc_level,omitempty"`          // low | medium | high
	CBDLevel          string   `json:"cbd_level,omitempty"`          // low | medium | high
	PrimaryAilments   []string `json:"primary_ailments,omitempty"`   // Any of these ailments
	PrimaryEffects    []string `json:"primary_effects,omitempty"`    // Any of these effects
	ConsumptionMethod []string `json:"consumption_method,omitempty"` // Any of these methods
	ExperienceLevel   []string `json:"experience_level,omitempty"`   // Any of these levels
	TimeOfUse         string   `json:"time_of_use,omitempty"`        // daytime | evening | anytime
	ExcludePubMed     bool     `json:"exclude_pubmed,omitempty"`     // For frontend: exclude PubMed sources
}

// BulkOperation represents a single operation in a bulk request
type BulkOperation struct {
	Action   string            `json:"action"` // index | update | delete
	Document KnowledgeDocument `json:"document"`
}

// BulkResponse represents the response from a bulk operation
type BulkResponse struct {
	Took      int64              `json:"took"`
	Errors    bool               `json:"errors"`
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
	Items     []BulkItemResponse `json:"items,omitempty"`
}

// BulkItemResponse represents a single item's response in a bulk operation
type BulkItemResponse struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

// HealthStatus represents the health status of the OpenSearch cluster
type HealthStatus struct {
	Status      string `json:"status"` // green | yellow | red
	ClusterName string `json:"cluster_name"`
	Available   bool   `json:"available"`
	Message     string `json:"message,omitempty"`
}

// Source type constants
const (
	SourceTypeDuckDuckGo    = "duckduckgo"
	SourceTypePubMed        = "pubmed"
	SourceTypeUserGenerated = "user_generated"
	SourceTypeBrand         = "brand"
)

// Content type constants
const (
	ContentTypeEducational = "educational"
	ContentTypeResearch    = "research"
	ContentTypeProductInfo = "product_info"
)

// THC/CBD level constants
const (
	LevelLow    = "low"
	LevelMedium = "medium"
	LevelHigh   = "high"
)

// Time of use constants
const (
	TimeOfUseDaytime = "daytime"
	TimeOfUseEvening = "evening"
	TimeOfUseAnytime = "anytime"
)
