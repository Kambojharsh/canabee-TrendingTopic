# System Architecture & Implementation Plan
## Cannabis Knowledge Intelligence Platform - New Flow

**Last Updated:** January 2, 2026  
**Version:** 2.0 (Revised after codebase analysis)

---

## Codebase Analysis Summary

### Current Architecture (Discovered)

| Component | Technology | Location |
|-----------|------------|----------|
| **Main Backend** | Go | `cannabee/pkg/` |
| **WebSocket Chat** | Go + Gorilla | `pkg/websocket/client.go` |
| **OpenAI Integration** | Go | `pkg/chat/openai.go` |
| **Product Search** | Qdrant | `pkg/qdrant/client.go` |
| **Knowledge Retrieval** | Qdrant | `pkg/chat/knowledge.go` |
| **Product Recommendations** | Go + Qdrant | `pkg/products/service.go` |
| **Database** | PostgreSQL + sqlc | `pkg/db/` |
| **Python POC** | Streamlit | `streamlit_chat_app.py` (separate server) |

### Existing Qdrant Collections
- `cannabis_products_staging` - Product catalog vectors
- `cannabis_knowledge_documents` - Existing knowledge base

### Proposed Addition
- **AWS OpenSearch** - New educational knowledge base (DuckDuckGo + PubMed sourced)

---

## Executive Summary

This is a **foundational architecture change** that transforms the system from an "LLM-first" approach to a "Knowledge-first" approach. The key shift is:

| Aspect | OLD | NEW |
|--------|-----|-----|
| Primary Intelligence | OpenAI (generates answers) | VectorDB (retrieves knowledge) |
| Learning | None | Permanent (every search enriches DB) |
| External Search | Ad-hoc, results discarded | Fallback, results stored |
| Source Handling | Mixed | Separated (DuckDuckGo → frontend, PubMed → backend) |
| Knowledge Store | Qdrant (limited) | AWS OpenSearch (scalable) |

---

## Phase 1: AWS OpenSearch Infrastructure

### 1.1 AWS OpenSearch Serverless Setup
**Effort: Medium | Priority: Critical | Dependency: None | Language: Terraform/CloudFormation**

```
Tasks:
├── Provision AWS OpenSearch Serverless cluster
├── Configure access policies and IAM roles
├── Create collection: cannabis-educational-knowledge
├── Define index mappings with metadata fields
├── Set up VPC endpoints (if needed)
└── Configure backup/restore policies
```

**Index Mapping Design:**
```json
{
  "mappings": {
    "properties": {
      "content_hash": { "type": "keyword" },
      "embedding_text": { "type": "text" },
      "embedding_vector": { 
        "type": "knn_vector", 
        "dimension": 1536,
        "method": { "name": "hnsw", "space_type": "cosinesimil" }
      },
      "source_type": { "type": "keyword" },
      "content_type": { "type": "keyword" },
      "category": { "type": "keyword" },
      "sub_category": { "type": "keyword" },
      "primary_ailments": { "type": "keyword" },
      "strain_type": { "type": "keyword" },
      "thc_level": { "type": "keyword" },
      "cbd_level": { "type": "keyword" },
      "effects": { "type": "keyword" },
      "consumption_method": { "type": "keyword" },
      "experience_level": { "type": "keyword" },
      "created_at": { "type": "date" },
      "updated_at": { "type": "date" },
      "ttl": { "type": "date" }
    }
  }
}
```

### 1.2 OpenSearch Go Client Integration
**Effort: Low | Priority: Critical | Dependency: 1.1 | Language: Go**

```
Tasks:
├── Add opensearch-go SDK to go.mod
├── Create pkg/opensearch/client.go
├── Implement connection pooling
├── Add health check endpoint
├── Create config in pkg/constants/
└── Add environment variable handling
```

**New File: `pkg/opensearch/client.go`**
```go
package opensearch

import (
    "context"
    opensearch "github.com/opensearch-project/opensearch-go/v2"
)

type Client struct {
    client *opensearch.Client
    index  string
}

type Config struct {
    Endpoint  string
    Region    string
    Index     string
    AccessKey string
    SecretKey string
}

func NewClient(cfg Config) (*Client, error) { ... }
func (c *Client) Search(ctx context.Context, vector []float32, filters map[string]interface{}, topK int) ([]SearchResult, error) { ... }
func (c *Client) Upsert(ctx context.Context, doc KnowledgeDocument) error { ... }
func (c *Client) BulkUpsert(ctx context.Context, docs []KnowledgeDocument) error { ... }
func (c *Client) Exists(ctx context.Context, contentHash string) (bool, error) { ... }
```

---

## Phase 2: Embedding & Hashing Service

### 2.1 Embedding Service Enhancement
**Effort: Low | Priority: Critical | Dependency: 1.2 | Language: Go**

Enhance existing `pkg/chat/openai.go` with:

```
Tasks:
├── Add text normalization function (lowercase, trim, dedupe spaces)
├── Add SHA256 content hashing function
├── Add batch embedding support
├── Implement embedding cache (Redis/in-memory)
└── Add embedding cost tracking (optional)
```

**Add to `pkg/chat/openai.go`:**
```go
// NormalizeTextForEmbedding prepares text for embedding (lowercase, trim, dedupe spaces)
func NormalizeTextForEmbedding(text string) string { ... }

// HashContent generates SHA256 hash for content deduplication
func HashContent(normalizedText string) string { ... }

// GenerateEmbeddingsBatch generates embeddings for multiple texts efficiently
func (oc *OpenAIClient) GenerateEmbeddingsBatch(ctx context.Context, texts []string) ([][]float32, error) { ... }
```

---

## Phase 3: Knowledge Ingestion Pipeline

### 3.1 Content Normalizer Service
**Effort: High | Priority: Critical | Dependency: 2.1 | Language: Go**

```
Tasks:
├── Create pkg/knowledge/normalizer.go
├── Parse DuckDuckGo search results
├── Parse PubMed article results
├── Extract metadata using lightweight LLM
│   ├── Ailments/use cases
│   ├── Product categories
│   ├── THC/CBD levels
│   ├── Effects and benefits
│   └── Experience level indicators
├── Map to cannabis-knowledge-metadata.json schema
├── Generate embedding_text (plain language, NO JSON)
└── Add source_type tagging
```

**New File: `pkg/knowledge/normalizer.go`**
```go
package knowledge

import (
    "cannabee/pkg/chat"
    "context"
)

type ContentNormalizer struct {
    openaiClient *chat.OpenAIClient
}

type KnowledgeRecord struct {
    ContentHash     string                 `json:"content_hash"`
    EmbeddingText   string                 `json:"embedding_text"`
    SourceType      string                 `json:"source_type"` // duckduckgo | pubmed
    ContentType     string                 `json:"content_type"` // educational | research | product_info
    Metadata        map[string]interface{} `json:"metadata"`
    RawContent      string                 `json:"raw_content"`
    SourceURL       string                 `json:"source_url"`
    CreatedAt       time.Time              `json:"created_at"`
}

func (cn *ContentNormalizer) NormalizeDuckDuckGoContent(ctx context.Context, results []DDGResult) ([]KnowledgeRecord, error) { ... }
func (cn *ContentNormalizer) NormalizePubMedContent(ctx context.Context, articles []PubMedArticle) ([]KnowledgeRecord, error) { ... }
func (cn *ContentNormalizer) ExtractMetadataWithLLM(ctx context.Context, text string) (map[string]interface{}, error) { ... }
```

**LLM Metadata Extraction Prompt:**
```go
const MetadataExtractionPrompt = `You are a cannabis content classifier. 
DO NOT generate educational content. ONLY extract structured metadata.

From the text below, extract:
- ailments: ["sleep", "pain", "anxiety", etc.] or []
- category: "flower" | "edible" | "vape" | "tincture" | "topical" | null
- thc_level: "low" | "medium" | "high" | null
- cbd_level: "low" | "medium" | "high" | null  
- effects: ["relaxed", "euphoric", "focused", etc.] or []
- consumption_method: ["smoke", "vape", "eat", "sublingual", "topical"] or []
- experience_level: ["beginner", "intermediate", "experienced"] or []
- confidence: "high" | "medium" | "low"

Return ONLY valid JSON. If information is unclear, use null or empty array.`
```

### 3.2 Knowledge Storage Service
**Effort: Medium | Priority: Critical | Dependency: 3.1, 1.2 | Language: Go**

```
Tasks:
├── Create pkg/knowledge/store.go
├── Implement upsert logic (check hash before embedding)
├── Implement batch storage for efficiency
├── Add source_type enforcement
├── Create audit trail in PostgreSQL
└── Add TTL/freshness policy
```

**New File: `pkg/knowledge/store.go`**
```go
package knowledge

import (
    "cannabee/pkg/chat"
    "cannabee/pkg/opensearch"
    "context"
    "database/sql"
)

type KnowledgeStore struct {
    opensearchClient *opensearch.Client
    openaiClient     *chat.OpenAIClient
    db               *sql.DB
}

func (ks *KnowledgeStore) Store(ctx context.Context, record KnowledgeRecord) error {
    // 1. Check if content_hash exists
    exists, err := ks.opensearchClient.Exists(ctx, record.ContentHash)
    if exists {
        log.Printf("Content already exists (hash: %s), skipping", record.ContentHash)
        return nil
    }
    
    // 2. Generate embedding
    embedding, err := ks.openaiClient.GenerateEmbeddings(ctx, record.EmbeddingText)
    
    // 3. Store in OpenSearch
    doc := opensearch.KnowledgeDocument{
        ContentHash:    record.ContentHash,
        EmbeddingText:  record.EmbeddingText,
        Vector:         embedding,
        SourceType:     record.SourceType,
        Metadata:       record.Metadata,
    }
    return ks.opensearchClient.Upsert(ctx, doc)
}

func (ks *KnowledgeStore) StoreBatch(ctx context.Context, records []KnowledgeRecord) error { ... }
```

### 3.3 PostgreSQL Audit Table
**Effort: Low | Priority: Medium | Dependency: 3.2 | Language: SQL**

**New Migration: `pkg/db/schema/024_knowledge_ingestion_audit.sql`**
```sql
CREATE TABLE IF NOT EXISTS knowledge_ingestion_audit (
    id SERIAL PRIMARY KEY,
    content_hash VARCHAR(64) NOT NULL,
    source_type VARCHAR(20) NOT NULL, -- duckduckgo, pubmed
    source_url TEXT,
    query_used TEXT,
    session_id VARCHAR(255),
    user_id VARCHAR(255),
    ingested_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata JSONB,
    UNIQUE(content_hash)
);

CREATE INDEX idx_knowledge_audit_source_type ON knowledge_ingestion_audit(source_type);
CREATE INDEX idx_knowledge_audit_ingested_at ON knowledge_ingestion_audit(ingested_at);
```

---

## Phase 4: Knowledge Retrieval Pipeline

### 4.1 Enhanced Knowledge Service
**Effort: Medium | Priority: Critical | Dependency: 1.2, 3.2 | Language: Go**

Refactor existing `pkg/chat/knowledge.go` to:

```
Tasks:
├── Add OpenSearch client alongside Qdrant
├── Implement dual-source retrieval (products from Qdrant, education from OpenSearch)
├── Add metadata filtering to queries
├── Implement relevance threshold logic
├── Add source_type filtering for frontend vs backend
└── Implement fallback trigger when no match found
```

**Enhanced `pkg/chat/knowledge.go`:**
```go
type KnowledgeService struct {
    qdrantClient     *qdrant.Client        // Products (existing)
    opensearchClient *opensearch.Client    // Educational knowledge (new)
    openaiClient     *chat.OpenAIClient
    knowledgeStore   *knowledge.KnowledgeStore
    fallbackHandler  *FallbackHandler
}

type RetrievalResult struct {
    Results              []KnowledgeResult
    ContextText          string
    HasRelevantKnowledge bool
    SourceTypes          []string // Which sources contributed
    FallbackTriggered    bool
}

func (ks *KnowledgeService) GetRelevantKnowledge(ctx context.Context, query string, filters map[string]interface{}) (*RetrievalResult, error) {
    // 1. Generate query embedding
    embedding, err := ks.openaiClient.GenerateEmbeddings(ctx, query)
    
    // 2. Search OpenSearch with metadata filters
    results, err := ks.opensearchClient.Search(ctx, embedding, filters, 10)
    
    // 3. Check if results meet relevance threshold
    if len(results) == 0 || results[0].Score < 0.75 {
        // Trigger fallback: external search
        return ks.fallbackHandler.Handle(ctx, query, filters)
    }
    
    // 4. Filter by source_type for frontend (exclude PubMed)
    filteredResults := ks.filterBySourceType(results, "duckduckgo")
    
    return &RetrievalResult{
        Results:              filteredResults,
        HasRelevantKnowledge: len(filteredResults) > 0,
        SourceTypes:          []string{"duckduckgo"},
    }, nil
}
```

### 4.2 Fallback Handler (External Search Orchestration)
**Effort: Medium | Priority: High | Dependency: 3.1, 3.2 | Language: Go**

```
Tasks:
├── Create pkg/knowledge/fallback.go
├── Integrate DuckDuckGo search (port from Python or use Go lib)
├── Integrate PubMed E-utilities API
├── Coordinate parallel searches
├── Send results through ingestion pipeline
├── Re-query after ingestion
└── Return newly stored knowledge
```

**New File: `pkg/knowledge/fallback.go`**
```go
package knowledge

import (
    "context"
    "sync"
)

type FallbackHandler struct {
    normalizer     *ContentNormalizer
    store          *KnowledgeStore
    ddgClient      *DuckDuckGoClient
    pubmedClient   *PubMedClient
}

func (fh *FallbackHandler) Handle(ctx context.Context, query string, filters map[string]interface{}) (*RetrievalResult, error) {
    log.Printf("=== Fallback Triggered for query: %s ===", query)
    
    var wg sync.WaitGroup
    var ddgResults []DDGResult
    var pubmedResults []PubMedArticle
    
    // Parallel external searches
    wg.Add(2)
    go func() {
        defer wg.Done()
        ddgResults, _ = fh.ddgClient.Search(ctx, query+" cannabis")
    }()
    go func() {
        defer wg.Done()
        pubmedResults, _ = fh.pubmedClient.Search(ctx, query)
    }()
    wg.Wait()
    
    // Normalize and store DuckDuckGo results
    ddgRecords, _ := fh.normalizer.NormalizeDuckDuckGoContent(ctx, ddgResults)
    for _, record := range ddgRecords {
        record.SourceType = "duckduckgo"
        fh.store.Store(ctx, record)
    }
    
    // Normalize and store PubMed results (backend only)
    pubmedRecords, _ := fh.normalizer.NormalizePubMedContent(ctx, pubmedResults)
    for _, record := range pubmedRecords {
        record.SourceType = "pubmed"
        fh.store.Store(ctx, record)
    }
    
    // Return DuckDuckGo results for frontend (PubMed stored but not returned)
    return &RetrievalResult{
        Results:              ddgRecords,
        HasRelevantKnowledge: len(ddgRecords) > 0,
        SourceTypes:          []string{"duckduckgo"},
        FallbackTriggered:    true,
    }, nil
}
```

### 4.3 DuckDuckGo Go Client
**Effort: Low | Priority: High | Dependency: None | Language: Go**

**New File: `pkg/knowledge/ddg_client.go`**
```go
package knowledge

import (
    "context"
    "net/http"
    "net/url"
)

type DuckDuckGoClient struct {
    httpClient *http.Client
}

type DDGResult struct {
    Title       string `json:"title"`
    Body        string `json:"body"`
    URL         string `json:"href"`
    IsReputable bool   `json:"is_reputable"`
}

func (c *DuckDuckGoClient) Search(ctx context.Context, query string) ([]DDGResult, error) { ... }
func (c *DuckDuckGoClient) isReputableSource(url string) bool { ... }
```

### 4.4 PubMed Go Client
**Effort: Low | Priority: High | Dependency: None | Language: Go**

**New File: `pkg/knowledge/pubmed_client.go`**
```go
package knowledge

import (
    "context"
    "encoding/xml"
    "net/http"
)

const NCBIEUtilsBase = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/"

type PubMedClient struct {
    httpClient *http.Client
    apiKey     string
}

type PubMedArticle struct {
    PMID     string   `json:"pmid"`
    Title    string   `json:"title"`
    Abstract string   `json:"abstract"`
    Authors  []string `json:"authors"`
    Journal  string   `json:"journal"`
    PubDate  string   `json:"pub_date"`
    DOI      string   `json:"doi"`
    PMCID    string   `json:"pmc_id"`
    URL      string   `json:"pubmed_url"`
}

func (c *PubMedClient) Search(ctx context.Context, query string) ([]PubMedArticle, error) { ... }
func (c *PubMedClient) FetchArticle(ctx context.Context, pmid string) (*PubMedArticle, error) { ... }
```

---

## Phase 5: Intent Normalization (Lightweight LLM)

### 5.1 Intent Extractor Enhancement
**Effort: Medium | Priority: High | Dependency: None | Language: Go**

Enhance `pkg/chat/openai.go` with lightweight intent extraction:

```
Tasks:
├── Create focused intent extraction prompt
├── Extract: ailment, form, flavor, experience_level
├── Handle ambiguous queries
├── Generate structured filters for OpenSearch
└── NO answer generation at this stage
```

**Add to `pkg/chat/openai.go`:**
```go
type UserIntent struct {
    Ailment         string   `json:"ailment"`
    FormFactor      string   `json:"form_factor"`
    FlavorPreference string  `json:"flavor_preference"`
    ExperienceLevel string   `json:"experience_level"`
    Effects         []string `json:"effects"`
    TimeOfUse       string   `json:"time_of_use"`
    Confidence      string   `json:"confidence"`
    MissingInfo     []string `json:"missing_info"`
}

func (oc *OpenAIClient) ExtractUserIntent(ctx context.Context, query string, conversationHistory []ChatMessage) (*UserIntent, error) {
    prompt := `You are an intent classifier for cannabis queries.
DO NOT generate educational content or answers.
ONLY extract structured information.

From the user query and conversation history, extract:
- ailment: What condition/use case? (sleep, pain, anxiety, focus, etc.) or null
- form_factor: What form? (edible, flower, vape, tincture, topical) or null
- flavor_preference: Any flavor mentioned? or null
- experience_level: beginner/intermediate/experienced or null
- effects: ["relaxed", "euphoric", "focused", etc.] or []
- time_of_use: daytime/evening/anytime or null
- confidence: high/medium/low
- missing_info: ["experience_level", "form_factor"] - what's needed for good recommendations

Return ONLY valid JSON.`
    
    // ... implementation
}
```

---

## Phase 6: Response Generation Refactor

### 6.1 Knowledge-Based Response Builder
**Effort: Medium | Priority: High | Dependency: 4.1, 5.1 | Language: Go**

Refactor WebSocket response flow in `pkg/websocket/client.go`:

```
Tasks:
├── Integrate new knowledge retrieval pipeline
├── Build response from OpenSearch knowledge (NOT LLM generation)
├── Implement source separation:
│   ├── Frontend: Use ONLY DuckDuckGo-sourced content
│   └── Backend/Bartender API: Include PubMed content
├── Format knowledge citations properly
├── Keep product recommendations separate (existing flow)
└── Maintain conversational context
```

**Modify `streamAIResponseWithHistoryContext` in `pkg/websocket/client.go`:**
```go
func (c *Client) streamAIResponseWithHistoryContext(messages []chat.ChatMessage, historyContext *chat.UserSessionContext, isGuest bool) {
    // 1. Extract user intent (lightweight LLM)
    intent, err := c.hub.openai.ExtractUserIntent(ctx, lastUserMessage, messages)
    
    // 2. Build filters from intent
    filters := buildFiltersFromIntent(intent)
    
    // 3. Query knowledge base (OpenSearch first, fallback if needed)
    knowledgeResponse, err := c.hub.KnowledgeService.GetRelevantKnowledge(ctx, lastUserMessage, filters)
    
    // 4. If fallback was triggered, knowledge is now stored and available
    if knowledgeResponse.FallbackTriggered {
        log.Printf("New knowledge ingested from external search")
    }
    
    // 5. Build response from retrieved knowledge (not LLM generation)
    if knowledgeResponse.HasRelevantKnowledge {
        // Use knowledge to inform response, not generate from scratch
        enhancedMessages := c.enhanceMessagesWithRetrievedKnowledge(messages, knowledgeResponse)
        stream, err = c.hub.openai.GetStreamResponseWithKnowledge(ctx, enhancedMessages, knowledgeResponse)
    }
    
    // ... rest of existing flow (product recommendations, suggestions)
}
```

---

## Phase 7: Configuration & Integration

### 7.1 Configuration Updates
**Effort: Low | Priority: Medium | Dependency: All | Language: Go**

**Update `pkg/constants/config.go`:**
```go
// OpenSearch Configuration
const (
    OpenSearchEndpoint = "OPENSEARCH_ENDPOINT"
    OpenSearchIndex    = "cannabis-educational-knowledge"
    OpenSearchRegion   = "us-east-1"
)

// Knowledge Retrieval Configuration
const (
    KnowledgeRetrievalThreshold = 0.75
    KnowledgeMaxResults         = 10
    FallbackEnabled             = true
)

// Source Type Constants
const (
    SourceTypeDuckDuckGo = "duckduckgo"
    SourceTypePubMed     = "pubmed"
    SourceTypeUserGen    = "user_generated"
    SourceTypeBrand      = "brand"
)
```

### 7.2 Environment Variables
```bash
# AWS OpenSearch
OPENSEARCH_ENDPOINT=https://xxx.us-east-1.aoss.amazonaws.com
OPENSEARCH_INDEX=cannabis-educational-knowledge
AWS_ACCESS_KEY_ID=xxx
AWS_SECRET_ACCESS_KEY=xxx
AWS_REGION=us-east-1

# PubMed
PUBMED_API_KEY=xxx  # Optional, increases rate limit

# Feature Flags
KNOWLEDGE_FALLBACK_ENABLED=true
KNOWLEDGE_RETRIEVAL_THRESHOLD=0.75
```

---

## Implementation Order (Revised)

```
Week 1-2: Infrastructure
├── 1.1 AWS OpenSearch Serverless Setup (Terraform)
├── 1.2 OpenSearch Go Client Integration
├── 2.1 Embedding Service Enhancement
└── Create integration tests

Week 3-4: Ingestion Pipeline
├── 3.1 Content Normalizer Service
├── 3.2 Knowledge Storage Service
├── 3.3 PostgreSQL Audit Table
├── 4.3 DuckDuckGo Go Client
├── 4.4 PubMed Go Client
└── Manual ingestion testing

Week 5: Retrieval Pipeline
├── 4.1 Enhanced Knowledge Service
├── 4.2 Fallback Handler
└── Test search quality

Week 6: Intent & Response
├── 5.1 Intent Extractor Enhancement
├── 6.1 Knowledge-Based Response Builder
└── Integration testing

Week 7-8: Integration & Testing
├── 7.1 Configuration Updates
├── 7.2 Environment Variables
├── Refactor WebSocket flow
├── End-to-end testing
└── Performance tuning
```

---

## New File Structure (Go Backend)

```
cannabee/pkg/
├── opensearch/
│   ├── client.go           # OpenSearch connection & operations
│   └── models.go           # Document types
├── knowledge/
│   ├── normalizer.go       # Content → Schema transformation
│   ├── store.go            # OpenSearch write operations
│   ├── fallback.go         # External search orchestration
│   ├── ddg_client.go       # DuckDuckGo search client
│   └── pubmed_client.go    # PubMed E-utilities client
├── chat/
│   ├── openai.go           # (enhanced) + intent extraction
│   ├── knowledge.go        # (enhanced) + OpenSearch integration
│   └── ...
├── qdrant/
│   └── client.go           # (unchanged) - Products only
├── products/
│   └── service.go          # (unchanged) - Product recommendations
├── websocket/
│   └── client.go           # (refactored) - Knowledge-first flow
├── constants/
│   └── config.go           # (enhanced) - OpenSearch config
└── db/
    ├── schema/
    │   └── 024_knowledge_ingestion_audit.sql
    └── queries/
        └── knowledge_audit.sql
```

---

## Guardrails Implementation Checklist

```
✅ MUST DO:
├── [ ] Query OpenSearch BEFORE external search
├── [ ] Store ALL new knowledge after external search
├── [ ] Tag source_type (duckduckgo | pubmed) on every record
├── [ ] Hash embedding content before storage
├── [ ] Separate frontend (DuckDuckGo) vs backend (PubMed) access
├── [ ] Keep Products in Qdrant (no migration needed)
└── [ ] Preserve existing WebSocket message format

❌ MUST NOT:
├── [ ] Do NOT show PubMed content to frontend users
├── [ ] Do NOT generate summaries from PubMed for users
├── [ ] Do NOT rely on OpenAI for factual cannabis education
├── [ ] Do NOT embed JSON metadata (only plain text)
├── [ ] Do NOT break existing product recommendation flow
└── [ ] Do NOT modify Qdrant product collection
```

---

## Testing Strategy

```
Unit Tests (Go):
├── pkg/opensearch/client_test.go
├── pkg/knowledge/normalizer_test.go
├── pkg/knowledge/store_test.go
├── pkg/knowledge/fallback_test.go
├── pkg/knowledge/ddg_client_test.go
└── pkg/knowledge/pubmed_client_test.go

Integration Tests:
├── OpenSearch connection and CRUD
├── Full ingestion pipeline
├── Retrieval + fallback flow
├── End-to-end WebSocket flow
└── Source separation validation

Performance Tests:
├── Embedding generation latency
├── OpenSearch query latency
├── Fallback + ingestion cycle time
└── Memory usage under load
```

---

## Migration Strategy

### Phase A: Shadow Mode (Week 1-2 post-implementation)
- New knowledge system runs in parallel
- Existing Qdrant knowledge continues serving
- Log comparison metrics

### Phase B: Gradual Rollout (Week 3-4)
- 10% → 50% → 100% traffic to OpenSearch
- Monitor error rates and latency
- Compare response quality

### Phase C: Deprecation (Week 5+)
- Remove Qdrant knowledge queries (keep products)
- Archive `cannabis_knowledge_documents` collection
- Full OpenSearch for educational content

---

## Questions Resolved

| Question | Decision |
|----------|----------|
| VectorDB Choice | AWS OpenSearch Serverless |
| Embedding Model | OpenAI text-embedding-ada-002 (already used) |
| Backend Language | Go (existing) |
| Products Storage | Qdrant (unchanged) |
| Knowledge Storage | AWS OpenSearch (new) |
| Python POC | Keep separate, update later if needed |

---

## Flow Diagrams

### New Query Flow (Go Backend)
```
User Message (WebSocket)
    │
    ▼
┌─────────────────┐
│ Intent Extractor│ (Lightweight LLM)
│ - Extract intent│
│ - Build filters │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ OpenSearch Query│
│ - Semantic search│
│ - Metadata filter│
└────────┬────────┘
         │
    ┌────┴────┐
    │ Match?  │
    │ (≥0.75) │
    └────┬────┘
         │
    ┌────┴────┐
    │         │
   YES        NO
    │         │
    ▼         ▼
┌───────┐  ┌──────────────┐
│Return │  │Fallback:     │
│Knowledge│ │DuckDuckGo +  │
│(DDG only)│ │PubMed Search │
└───────┘  └──────┬───────┘
                  │
                  ▼
           ┌──────────────┐
           │Normalize &   │
           │Store in      │
           │OpenSearch    │
           └──────┬───────┘
                  │
                  ▼
           ┌──────────────┐
           │Return DDG    │
           │Knowledge     │
           │(PubMed stored│
           │but not shown)│
           └──────────────┘
```

### Knowledge Ingestion Flow
```
Raw Content (DuckDuckGo/PubMed)
    │
    ▼
┌─────────────────┐
│ Content Parser  │
│ - Extract text  │
│ - Clean/normalize│
│ - Tag source_type│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ LLM Metadata    │
│ Extraction      │
│ - Ailments      │
│ - Categories    │
│ - Effects       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Generate        │
│ embedding_text  │
│ (plain language)│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Hash Content    │
│ SHA256(text)    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Check Exists?   │
│ (OpenSearch)    │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
   YES        NO
    │         │
    ▼         ▼
 [Skip]    ┌──────────────┐
           │ Generate     │
           │ Embedding    │
           │ (OpenAI)     │
           └──────┬───────┘
                  │
                  ▼
           ┌──────────────┐
           │ Store in     │
           │ OpenSearch   │
           │ + Audit Log  │
           └──────────────┘
```

---

*Document Created: January 2, 2026*  
*Last Updated: January 2, 2026*  
*Version: 2.0 (Go-centric architecture)*
