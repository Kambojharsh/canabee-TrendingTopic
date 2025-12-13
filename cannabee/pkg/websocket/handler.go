package websocket

import (
	"cannabee/pkg/chat"
	"cannabee/pkg/products"
	"database/sql"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

type Hub struct {
	clients                      map[*Client]bool
	broadcast                    chan []byte
	register                     chan *Client
	unregister                   chan *Client
	db                           *sql.DB
	openai                       *chat.OpenAIClient
	contextService               *chat.ContextService
	ProductRecommendationService *products.ProductRecommendationService
	KnowledgeService             *chat.KnowledgeService
	SuggestionService            *chat.SuggestionService
}

type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	send      chan []byte
	userID    string
	sessionID string
	closed    bool
	mu        sync.RWMutex
	isGuest   bool // Flag to indicate if this is a guest user
}

type Message struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	SessionID string `json:"session_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Timestamp string `json:"timestamp"`
}

func NewHub(db *sql.DB, openaiClient *chat.OpenAIClient, apiKey string) *Hub {
	log.Printf("=== Initializing WebSocket Hub ===")

	contextService := chat.NewContextService(db, openaiClient)
	log.Printf("Context service initialized successfully")

	// Initialize product recommendation service
	log.Printf("Initializing Product Recommendation Service...")
	productService, err := products.NewProductRecommendationService(openaiClient, db)
	if err != nil {
		log.Printf("WARNING: Failed to initialize product recommendation service: %v", err)
		log.Printf("Product recommendations will be DISABLED")
		productService = nil
	} else {
		log.Printf("Product Recommendation Service initialized successfully")
	}

	// Initialize knowledge service
	log.Printf("Initializing Cannabis Knowledge Service...")
	knowledgeService, err := chat.NewKnowledgeService(openaiClient)
	if err != nil {
		log.Printf("WARNING: Failed to initialize cannabis knowledge service: %v", err)
		log.Printf("Cannabis knowledge base will be DISABLED - AI will use fallback responses")
		knowledgeService = nil
	} else {
		log.Printf("Cannabis Knowledge Service initialized successfully")
	}

	// Initialize suggestion service
	log.Printf("Initializing Suggestion Service...")
	suggestionService := chat.NewSuggestionService(apiKey)
	log.Printf("Suggestion Service initialized successfully")

	hub := &Hub{
		clients:                      make(map[*Client]bool),
		register:                     make(chan *Client),
		unregister:                   make(chan *Client),
		db:                           db,
		openai:                       openaiClient,
		contextService:               contextService,
		ProductRecommendationService: productService,
		KnowledgeService:             knowledgeService,
		SuggestionService:            suggestionService,
	}

	log.Printf("=== WebSocket Hub Initialization Complete ===")
	log.Printf("Services Status:")
	log.Printf("- Context Service: ENABLED")
	log.Printf("- Product Recommendations: %s", func() string {
		if productService != nil {
			return "ENABLED"
		}
		return "DISABLED"
	}())
	log.Printf("- Cannabis Knowledge Base: %s", func() string {
		if knowledgeService != nil {
			return "ENABLED"
		}
		return "DISABLED"
	}())
	log.Printf("- Suggestion Service: ENABLED")

	return hub
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			log.Printf("Client connected. Total clients: %d", len(h.clients))

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.safeClose()
				log.Printf("DEBUG: Client unregistered and marked as closed for session %s. Total clients: %d", client.sessionID, len(h.clients))
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					client.safeClose()
					delete(h.clients, client)
				}
			}
		}
	}
}
