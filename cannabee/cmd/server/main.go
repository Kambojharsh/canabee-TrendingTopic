package main

import (
	"cannabee/pkg/auth"
	"cannabee/pkg/chat"
	"cannabee/pkg/database"
	"cannabee/pkg/handlers"
	"cannabee/pkg/websocket"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

func main() {
	// Load environment variables from .env file (optional, falls back to system env vars)
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found or error loading it: %v (using system environment variables)", err)
	}

	// Get OpenAI API key from environment variable
	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	if openaiAPIKey == "" {
		log.Println("Warning: OPENAI_API_KEY not set. Using placeholder. Set this environment variable.")
		openaiAPIKey = "your-openai-api-key-here"
	}

	// Initialize database connection
	db, err := database.NewConnection()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize authentication
	cognitoAuth, err := auth.NewCognitoAuth()
	if err != nil {
		log.Fatalf("Failed to initialize Cognito authentication: %v", err)
	}

	// Initialize components
	openaiClient := chat.NewOpenAIClient(openaiAPIKey)
	wsHub := websocket.NewHub(db.DB, openaiClient, openaiAPIKey)
	server := handlers.NewServer(db.DB, wsHub, openaiClient)

	// Start WebSocket hub
	go wsHub.Run()

	// Set up routes
	router := mux.NewRouter()

	// CORS middleware - Apply BEFORE setting up routes
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "https://localhost:3000", "*"}, // Add localhost specifically
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
		Debug:            true, // Enable debug logging for CORS
	})

	log.Printf("🌐 [CORS] CORS middleware configured:")
	log.Printf("🌐 [CORS] Allowed Origins: http://localhost:3000, https://localhost:3000, *")
	log.Printf("🌐 [CORS] Allowed Methods: GET, POST, PUT, DELETE, OPTIONS")
	log.Printf("🌐 [CORS] Allowed Headers: *")
	log.Printf("🌐 [CORS] Allow Credentials: true")
	log.Printf("🌐 [CORS] Debug Mode: true")

	// Apply CORS middleware to the router
	router.Use(c.Handler)

	// Public routes (no authentication required)
	router.HandleFunc("/health", auth.HealthCheckHandler).Methods("GET", "OPTIONS")
	router.HandleFunc("/cors-test", server.CORSTestHandler).Methods("GET", "POST", "OPTIONS") // CORS test endpoint

	// Protected routes (authentication required)
	log.Println("🔐 [SERVER] Setting up protected routes with JWT authentication middleware")
	protected := router.NewRoute().Subrouter()
	protected.Use(cognitoAuth.AuthMiddleware)
	log.Println("🔐 [SERVER] JWT authentication middleware applied to protected routes")

	// API routes with authentication
	log.Println("🔐 [SERVER] Registering protected API routes:")
	protected.HandleFunc("/sessions", server.GetSessionsHandler).Methods("GET", "OPTIONS")
	log.Println("  ✅ GET /sessions - Get user's chat sessions")
	protected.HandleFunc("/sessions", server.CreateSessionHandler).Methods("POST", "OPTIONS")
	log.Println("  ✅ POST /sessions - Create new chat session")
	protected.HandleFunc("/sessions/{sessionId}", server.GetSessionHandler).Methods("GET", "OPTIONS")
	log.Println("  ✅ GET /sessions/{sessionId} - Get specific session details")
	protected.HandleFunc("/sessions/{sessionId}", server.DeleteSessionHandler).Methods("DELETE", "OPTIONS")
	log.Println("  ✅ DELETE /sessions/{sessionId} - Delete session")
	protected.HandleFunc("/sessions/{sessionId}/summary", server.GetSessionSummaryHandler).Methods("GET", "OPTIONS")
	log.Println("  ✅ GET /sessions/{sessionId}/summary - Get session summary")
	protected.HandleFunc("/sessions/{sessionId}/tags", server.GetSessionTagsHandler).Methods("GET", "OPTIONS")
	log.Println("  ✅ GET /sessions/{sessionId}/tags - Get session tags")
	protected.HandleFunc("/sessions/{sessionId}/recommendations", server.GetSessionRecommendationsHandler).Methods("GET", "OPTIONS")
	log.Println("  ✅ GET /sessions/{sessionId}/recommendations - Get similar sessions")
	protected.HandleFunc("/analytics", server.GetUserAnalyticsHandler).Methods("GET", "OPTIONS")
	log.Println("  ✅ GET /analytics - Get user session analytics")
	protected.HandleFunc("/tags/popular", server.GetPopularTagsHandler).Methods("GET", "OPTIONS")
	log.Println("  ✅ GET /tags/popular - Get popular tags across all sessions")
	protected.HandleFunc("/chat/{sessionId}/messages", server.GetChatMessagesHandler).Methods("GET", "OPTIONS")
	log.Println("  ✅ GET /chat/{sessionId}/messages - Get chat messages for a session")
	protected.HandleFunc("/user/database-id", server.GetDatabaseUserIDHandler).Methods("GET", "OPTIONS")
	log.Println("  ✅ GET /user/database-id - Get database user ID for authenticated user")
	protected.HandleFunc("/sessions/{sessionId}/location", server.SetSessionLocationHandler).Methods("PUT", "OPTIONS")
	log.Println("  ✅ PUT /sessions/{sessionId}/location - Set session location and radius")

	// WebSocket endpoint (public route - authentication handled in handler)
	router.HandleFunc("/chat/{sessionId}", server.WebSocketHandler).Methods("GET", "OPTIONS")
	log.Println("  ✅ GET /chat/{sessionId} - WebSocket endpoint for chat (public route)")

	// API routes - Guest Users (AWS Cognito Identity Pool)
	router.HandleFunc("/guest/sessions", server.GetGuestSessionsHandler).Methods("GET")
	router.HandleFunc("/guest/sessions", server.CreateGuestSessionHandler).Methods("POST")
	router.HandleFunc("/guest/sessions/{sessionId}", server.GetGuestSessionHandler).Methods("GET")
	router.HandleFunc("/guest/sessions/{sessionId}", server.DeleteGuestSessionHandler).Methods("DELETE")
	router.HandleFunc("/guest/sessions/{sessionId}/location", server.SetGuestSessionLocationHandler).Methods("PUT", "OPTIONS")
	log.Println("  ✅ PUT /guest/sessions/{sessionId}/location - Set guest session location and radius")
	router.HandleFunc("/guest/chat/{sessionId}/messages", server.GetGuestChatMessagesHandler).Methods("GET")
	router.HandleFunc("/guest/chat/{sessionId}", server.GuestWebSocketHandler).Methods("GET", "OPTIONS")
	// Start server
	log.Println("🚀 [SERVER] Starting Cannabee AI Chat Server with JWT Authentication")
	log.Println("🔐 [SERVER] JWT authentication enabled with Cognito integration")
	log.Println("🌐 [SERVER] Server starting on :8080")
	log.Println()
	log.Println("🔓 Public Endpoints:")
	log.Println("  GET /health - Health check (no auth required)")
	log.Println("  GET /cors-test - CORS test endpoint (no auth required)")
	log.Println("  POST /cors-test - CORS test endpoint (no auth required)")
	log.Println("  OPTIONS /cors-test - CORS preflight test (no auth required)")
	log.Println("  GET /chat/{sessionId}?token=JWT_TOKEN - WebSocket endpoint for chat (JWT in query parameter)")
	log.Println()
	log.Println("🔒 Protected Endpoints (JWT Required):")
	log.Println("  GET /sessions - Get user's chat sessions")
	log.Println("  POST /sessions - Create new chat session")
	log.Println("  GET /sessions/{sessionId} - Get specific session details")
	log.Println("  DELETE /sessions/{sessionId} - Delete session")
	log.Println("  GET /sessions/{sessionId}/summary - Get session summary")
	log.Println("  GET /sessions/{sessionId}/tags - Get session tags")
	log.Println("  GET /sessions/{sessionId}/recommendations - Get similar sessions")
	log.Println("  GET /analytics - Get user session analytics")
	log.Println("  GET /tags/popular - Get popular tags across all sessions")
	log.Println("  GET /chat/{sessionId}/messages - Get chat messages for a session")
	log.Println("  GET /user/database-id - Get database user ID for authenticated user")
	log.Println()
	log.Println("🔑 Authentication:")
	log.Println("  - All protected endpoints require 'Authorization: Bearer <JWT_TOKEN>' header")
	log.Println("  - JWT tokens must be issued by AWS Cognito")
	log.Println("  - WebSocket connections (public route) require 'token' query parameter with JWT")
	log.Println("  - User info is extracted from validated JWT claims")
	log.Println()
	log.Println("🧠 Context-Aware Features (Automatic):")
	log.Println("  - Personalized greetings based on user's last 3 sessions (on session start)")
	log.Println("  - AI responses enhanced with previous session context")
	log.Println("  - Automatic tag cleanup (stack-like structure, keeps last 3)")
	log.Println("  - Session summaries and tags auto-generated on disconnect")
	log.Println()
	log.Println("📋 Environment Variables Required:")
	log.Println("  - AWS_COGNITO_USER_POOL_ID: Your Cognito User Pool ID")
	log.Println("  - AWS_REGION: AWS region where your Cognito User Pool is located")
	log.Println("  - OPENAI_API_KEY: OpenAI API key for chat functionality")
	log.Println()
	log.Println("Example usage:")
	log.Println("1. Health check: curl http://localhost:8080/health")
	log.Println("2. CORS test: curl http://localhost:8080/cors-test")
	log.Println("3. Create session: curl -X POST http://localhost:8080/sessions -H 'Authorization: Bearer <JWT>' -d '{\"metadata\":\"My Chat\"}'")
	log.Println("4. Get sessions: curl http://localhost:8080/sessions -H 'Authorization: Bearer <JWT>'")
	log.Println("5. Get messages: curl http://localhost:8080/chat/{sessionId}/messages -H 'Authorization: Bearer <JWT>'")
	log.Println("6. WebSocket: ws://localhost:8080/chat/{sessionId}?token=<JWT>")
	log.Println("7. Get analytics: curl http://localhost:8080/analytics -H 'Authorization: Bearer <JWT>'")

	// TLS certificate paths
	// certFile := "/home/ec2-user/services/cert/certificate.crt"
	// keyFile := "/home/ec2-user/services/cert/private.key"

	// if err := http.ListenAndServeTLS(":443", certFile, keyFile, handler); err != nil {
	// 	log.Fatalf("TLS Server failed to start: %v", err)
	// }

	// cert file is not there, start on 8080
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
