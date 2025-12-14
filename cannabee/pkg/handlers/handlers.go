package handlers

import (
	"cannabee/pkg/auth"
	"cannabee/pkg/chat"
	"cannabee/pkg/db"
	"cannabee/pkg/recommendations"
	"cannabee/pkg/websocket"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Server struct {
	db                    *sql.DB
	wsHub                 *websocket.Hub
	openaiClient          *chat.OpenAIClient
	recommendationService *recommendations.RecommendationService
}

type SessionResponse struct {
	SessionID        string `json:"session_id"`
	UserID           string `json:"user_id"`
	SessionStartTime string `json:"session_start_time"`
	LastAccessedAt   string `json:"last_accessed_at"`
	IsActive         bool   `json:"is_active"`
	Metadata         string `json:"metadata"`
}

type CreateSessionRequest struct {
	Metadata      interface{}            `json:"metadata,omitempty"`
	SessionType   string                 `json:"session_type,omitempty"`   // "my_journey" or "guide_me" or "ask_bee"
	SessionConfig map[string]interface{} `json:"session_config,omitempty"` // Session-specific config
	// NEW: Location fields
	Latitude   *float64 `json:"latitude,omitempty"`
	Longitude  *float64 `json:"longitude,omitempty"`
	RadiusMile *float64 `json:"radius_mile,omitempty"`
	Address    string   `json:"address,omitempty"`
}

type CreateSessionResponse struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	Metadata  string `json:"metadata"`
}

// Location structure for session metadata
type SessionLocation struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	RadiusMile float64 `json:"radius_mile"`
	Address    string  `json:"address,omitempty"`
	SetAt      string  `json:"set_at"` // Timestamp when location was set
}

// Enhanced session metadata structure
type SessionMetadata struct {
	SessionType   string                 `json:"session_type,omitempty"`
	SessionConfig map[string]interface{} `json:"session_config,omitempty"`
	UserLocation  *SessionLocation       `json:"user_location,omitempty"`
	// Additional metadata fields can be added here
}

func NewServer(database *sql.DB, wsHub *websocket.Hub, openaiClient *chat.OpenAIClient) *Server {
	return &Server{
		db:                    database,
		wsHub:                 wsHub,
		openaiClient:          openaiClient,
		recommendationService: recommendations.NewRecommendationService(database),
	}
}

// CORS test handler for debugging
func (s *Server) CORSTestHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("🔍 [CORS TEST] Request received: %s %s", r.Method, r.URL.Path)
	log.Printf("🔍 [CORS TEST] Origin: %s", r.Header.Get("Origin"))
	log.Printf("🔍 [CORS TEST] Method: %s", r.Method)
	log.Printf("🔍 [CORS TEST] Headers: %v", r.Header)

	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "CORS test successful",
		"method":    r.Method,
		"origin":    r.Header.Get("Origin"),
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// CORS preflight handler
func (s *Server) CORSPreflightHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.WriteHeader(http.StatusOK)
}

// GetSessionsHandler retrieves all sessions for the authenticated user
func (s *Server) GetSessionsHandler(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Get the Cognito user ID from context
	cognitoUserID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Look up the database user ID based on Cognito user ID
	databaseUserID, err := auth.LookupDatabaseUserID(r.Context(), s.db, cognitoUserID)
	if err != nil {
		log.Printf("❌ [SESSIONS] Failed to lookup database user ID: %v", err)
		http.Error(w, "User not found in database", http.StatusNotFound)
		return
	}

	// Update the context with the database user ID
	ctx := auth.SetDatabaseUserIDInContext(r.Context(), databaseUserID)

	queries := db.New(s.db)
	sessions, err := queries.GetSessionsByUserID(ctx, databaseUserID)
	if err != nil {
		http.Error(w, "Failed to get sessions", http.StatusInternalServerError)
		return
	}

	var response []SessionResponse
	for _, session := range sessions {
		sessionResp := SessionResponse{
			SessionID:        session.SessionID,
			UserID:           session.UserID,                                          // Now directly a string, not nullable
			IsActive:         session.IsActive,                                        // Now directly a bool, not nullable
			SessionStartTime: session.SessionStartTime.Format("2006-01-02T15:04:05Z"), // Now directly time.Time
			LastAccessedAt:   session.LastAccessedAt.Format("2006-01-02T15:04:05Z"),   // Now directly time.Time
		}

		if session.Metadata.Valid {
			sessionResp.Metadata = session.Metadata.String
		}

		response = append(response, sessionResp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) CreateSessionHandler(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Get the Cognito user ID from context
	cognitoUserID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Look up the database user ID based on Cognito user ID
	databaseUserID, err := auth.LookupDatabaseUserID(r.Context(), s.db, cognitoUserID)
	if err != nil {
		log.Printf("❌ [CREATE SESSION] Failed to lookup database user ID: %v", err)
		http.Error(w, "User not found in database", http.StatusNotFound)
		return
	}

	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Generate UUID for session
	sessionID := uuid.New().String()
	now := time.Now()

	// Build enhanced session metadata
	sessionMetadata := SessionMetadata{}

	// Handle existing metadata
	if req.Metadata != nil {
		// Try to merge existing metadata
		if metadataBytes, err := json.Marshal(req.Metadata); err == nil {
			var existingMetadata map[string]interface{}
			if json.Unmarshal(metadataBytes, &existingMetadata) == nil {
				// Preserve any existing fields not handled by SessionMetadata
				if sessionType, ok := existingMetadata["session_type"].(string); ok {
					sessionMetadata.SessionType = sessionType
				}
				if sessionConfig, ok := existingMetadata["session_config"].(map[string]interface{}); ok {
					sessionMetadata.SessionConfig = sessionConfig
				}
			}
		}
	}

	// Add session type and config
	if req.SessionType != "" {
		sessionMetadata.SessionType = req.SessionType
	}
	if req.SessionConfig != nil {
		sessionMetadata.SessionConfig = req.SessionConfig
	}

	// Add location data if provided
	if req.Latitude != nil && req.Longitude != nil {
		sessionMetadata.UserLocation = &SessionLocation{
			Latitude:   *req.Latitude,
			Longitude:  *req.Longitude,
			RadiusMile: 15.5, // Default radius
			Address:    req.Address,
			SetAt:      time.Now().Format(time.RFC3339),
		}

		// Override default radius if provided
		if req.RadiusMile != nil && *req.RadiusMile > 0 {
			sessionMetadata.UserLocation.RadiusMile = *req.RadiusMile
		}

		log.Printf("✅ [CREATE SESSION] Location added: lat=%.6f, lng=%.6f, radius=%.1f miles",
			*req.Latitude, *req.Longitude, sessionMetadata.UserLocation.RadiusMile)
	}

	// Convert to JSON for storage
	metadataBytes, err := json.Marshal(sessionMetadata)
	if err != nil {
		http.Error(w, "Error processing session data", http.StatusInternalServerError)
		return
	}
	metadataJSON := string(metadataBytes)
	metadataValid := true

	queries := db.New(s.db)
	err = queries.CreateSession(context.Background(), db.CreateSessionParams{
		SessionID:        sessionID,
		UserID:           databaseUserID, // Use database user ID, not Cognito ID
		SessionStartTime: now,            // Now directly time.Time, not sql.NullTime
		LastAccessedAt:   now,            // Now directly time.Time, not sql.NullTime
		IsActive:         true,           // Now directly bool, not sql.NullBool
		Metadata:         sql.NullString{String: metadataJSON, Valid: metadataValid},
		Summary:          sql.NullString{Valid: false},  // No summary at creation
		Tag:              sql.NullString{Valid: false},  // Tag will be set when session ends
		Latitude:         sql.NullFloat64{Valid: false}, // Location will be set when session ends
		Longitude:        sql.NullFloat64{Valid: false}, // Location will be set when session ends
	})
	if err != nil {
		log.Printf("Error creating session: %v", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	response := CreateSessionResponse{
		SessionID: sessionID,
		UserID:    databaseUserID, // Return database user ID
		Metadata:  metadataJSON,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) GetSessionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	queries := db.New(s.db)
	session, err := queries.GetSessionByID(context.Background(), sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	response := SessionResponse{
		SessionID:        session.SessionID,
		UserID:           session.UserID,                                          // Now directly a string, not nullable
		IsActive:         session.IsActive,                                        // Now directly a bool, not nullable
		SessionStartTime: session.SessionStartTime.Format("2006-01-02T15:04:05Z"), // Now directly time.Time
		LastAccessedAt:   session.LastAccessedAt.Format("2006-01-02T15:04:05Z"),   // Now directly time.Time
	}

	if session.Metadata.Valid {
		response.Metadata = session.Metadata.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) DeleteSessionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	queries := db.New(s.db)
	err := queries.DeleteSession(context.Background(), sessionID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
}

func (s *Server) WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	// Authentication will be handled by JWT token passed via query parameter for WebSocket
	// since WebSocket doesn't support custom headers during upgrade
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Authentication token required in query parameter", http.StatusUnauthorized)
		return
	}

	// Create a CognitoAuth instance for token validation
	cognitoAuth, err := auth.NewCognitoAuth()
	if err != nil {
		log.Printf("Error creating Cognito auth: %v", err)
		http.Error(w, "Authentication service unavailable", http.StatusInternalServerError)
		return
	}

	// Validate the JWT token
	claims, err := cognitoAuth.ValidateWebSocketToken(token)
	if err != nil {
		log.Printf("WebSocket JWT validation failed: %v", err)
		http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
		return
	}

	userID := claims.Username
	userEmail := claims.Email

	// Create a context with user information for database lookup
	ctx := context.Background()
	ctx = context.WithValue(ctx, auth.UserIDKey, userID)         // Cognito username
	ctx = context.WithValue(ctx, auth.UserEmailKey, userEmail)   // User email
	ctx = context.WithValue(ctx, auth.CognitoSubKey, claims.Sub) // Cognito sub

	// Look up the database user ID based on Cognito user ID
	databaseUserID, err := auth.LookupDatabaseUserID(ctx, s.db, userID)
	if err != nil {
		log.Printf("❌ [WEBSOCKET] Failed to lookup database user ID: %v", err)
		http.Error(w, "User not found in database", http.StatusUnauthorized)
		return
	}

	// Verify that the session exists and belongs to the user
	queries := db.New(s.db)
	session, err := queries.GetSessionByID(context.Background(), sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if session.UserID != databaseUserID { // Use database user ID for comparison
		http.Error(w, "Session does not belong to user", http.StatusForbidden)
		return
	}

	queries.UpdateSessionAccess(context.Background(), db.UpdateSessionAccessParams{
		IsActive:  true,
		SessionID: sessionID,
	})

	s.wsHub.HandleWebSocket(w, r, databaseUserID, sessionID) // Pass database user ID to WebSocket hub
}

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GetSessionSummaryHandler retrieves the summary for a specific session
func (s *Server) GetSessionSummaryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	queries := db.New(s.db)
	session, err := queries.GetSessionByID(context.Background(), sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Create response with summary from session
	response := map[string]interface{}{
		"session_id": session.SessionID,
		"summary":    session.Summary.String,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetSessionTagsHandler retrieves the tags for the user who owns the session
func (s *Server) GetSessionTagsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	queries := db.New(s.db)

	// First get the session to find the user
	session, err := queries.GetSessionByID(context.Background(), sessionID)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Get the user's tags
	tags, err := queries.GetUserTags(context.Background(), session.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}

// GetSessionRecommendationsHandler gets similar sessions based on tags
func (s *Server) GetSessionRecommendationsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	// Parse limit parameter (default to 10)
	limit := int32(10)
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limitInt, err := strconv.Atoi(limitStr); err == nil && limitInt > 0 && limitInt <= 50 {
			limit = int32(limitInt)
		}
	}

	recommendations, err := s.recommendationService.GetSimilarSessions(context.Background(), sessionID, limit)
	if err != nil {
		log.Printf("Error getting recommendations: %v", err)
		http.Error(w, "Failed to get recommendations", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recommendations)
}

// GetUserAnalyticsHandler provides analytics about a user's session patterns
func (s *Server) GetUserAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	// Get the Cognito user ID from context
	cognitoUserID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Look up the database user ID based on Cognito user ID
	databaseUserID, err := auth.LookupDatabaseUserID(r.Context(), s.db, cognitoUserID)
	if err != nil {
		log.Printf("❌ [ANALYTICS] Failed to lookup database user ID: %v", err)
		http.Error(w, "User not found in database", http.StatusNotFound)
		return
	}

	analytics, err := s.recommendationService.GetSessionAnalytics(context.Background(), databaseUserID)
	if err != nil {
		log.Printf("Error getting user analytics: %v", err)
		http.Error(w, "Failed to get analytics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analytics)
}

// GetPopularTagsHandler returns the most popular tags across all sessions
func (s *Server) GetPopularTagsHandler(w http.ResponseWriter, r *http.Request) {
	// Note: Popular tags functionality has been moved to user-level tags
	// For now, return empty array as this endpoint may need to be redesigned
	popularTags := []string{}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(popularTags)
}

// MessageResponse represents a message in the API response format
type MessageResponse struct {
	ID              int32       `json:"id"`
	Content         string      `json:"content"`
	UserType        string      `json:"user_type"`
	Timestamp       string      `json:"timestamp"`
	Recommendations interface{} `json:"recommendations,omitempty"`
}

// GetChatMessagesHandler retrieves all messages for a specific chat session
func (s *Server) GetChatMessagesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]

	if sessionID == "" {
		http.Error(w, "sessionId parameter required", http.StatusBadRequest)
		return
	}

	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Look up the database user ID based on Cognito user ID
	databaseUserID, err := auth.LookupDatabaseUserID(r.Context(), s.db, userID)
	if err != nil {
		log.Printf("❌ [CHAT MESSAGES] Failed to lookup database user ID: %v", err)
		http.Error(w, "User not found in database", http.StatusNotFound)
		return
	}

	// Validate session exists and belongs to user
	queries := db.New(s.db)
	session, err := queries.GetSessionByID(context.Background(), sessionID)
	if err != nil {
		log.Printf("Error getting session: %v", err)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	if session.UserID != databaseUserID { // Use database user ID for comparison
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get messages for the session
	chats, err := queries.GetChatsBySessionID(context.Background(), sessionID)
	if err != nil {
		log.Printf("Error getting chats: %v", err)
		http.Error(w, "Failed to get messages", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	messages := make([]MessageResponse, 0, len(chats))
	for _, chat := range chats {
		userType := "ai"
		if chat.IsUserMessage {
			userType = "user"
		}

		timestamp := ""
		if chat.CreatedAt.Valid {
			timestamp = chat.CreatedAt.Time.Format(time.RFC3339)
		}

		// Check if this is a stored product recommendation (JSON format)
		if !chat.IsUserMessage && strings.HasPrefix(strings.TrimSpace(chat.Message.String), "{") && strings.Contains(chat.Message.String, "product_recommendations") {
			log.Printf("Parsing stored product recommendation from database")

			// Parse the stored ProductStorageMessage format
			var storageMsg struct {
				Type            string      `json:"type"`
				Recommendations interface{} `json:"recommendations"`
				Timestamp       string      `json:"timestamp"`
			}

			if err := json.Unmarshal([]byte(chat.Message.String), &storageMsg); err != nil {
				log.Printf("ERROR: Failed to parse stored product recommendation: %v", err)
				// Fall back to regular message format
				messages = append(messages, MessageResponse{
					ID:        chat.ID,
					Content:   chat.Message.String,
					UserType:  userType,
					Timestamp: timestamp,
				})
				continue
			}

			log.Printf("Successfully parsed product recommendation from storage")

			// Create message with product recommendations
			messages = append(messages, MessageResponse{
				ID:              chat.ID,
				Content:         "Here are some products I found for you:",
				UserType:        userType,
				Timestamp:       timestamp,
				Recommendations: storageMsg.Recommendations,
			})
		} else {
			// Regular text message
			messages = append(messages, MessageResponse{
				ID:        chat.ID,
				Content:   chat.Message.String,
				UserType:  userType,
				Timestamp: timestamp,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// Helper to extract location from session metadata
func GetLocationFromSession(ctx context.Context, database *sql.DB, sessionID string) (*SessionLocation, error) {
	queries := db.New(database)
	session, err := queries.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if !session.Metadata.Valid {
		return nil, nil // No metadata
	}

	var metadata SessionMetadata
	if err := json.Unmarshal([]byte(session.Metadata.String), &metadata); err != nil {
		return nil, err
	}

	return metadata.UserLocation, nil
}

// Helper to update location in existing session
func UpdateSessionLocation(ctx context.Context, database *sql.DB, sessionID string, location *SessionLocation) error {
	queries := db.New(database)
	session, err := queries.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}

	var metadata SessionMetadata
	if session.Metadata.Valid {
		json.Unmarshal([]byte(session.Metadata.String), &metadata)
	}

	// Update location
	location.SetAt = time.Now().Format(time.RFC3339)
	metadata.UserLocation = location

	// Convert metadata back to JSON
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	// Update session metadata in database
	err = queries.UpdateSessionMetadata(ctx, db.UpdateSessionMetadataParams{
		SessionID: sessionID,
		Metadata:  sql.NullString{String: string(metadataBytes), Valid: true},
	})

	return err
}

// SetLocationRequest represents the request structure for setting session location
type SetLocationRequest struct {
	Latitude   float64 `json:"latitude" binding:"required"`
	Longitude  float64 `json:"longitude" binding:"required"`
	RadiusMile float64 `json:"radius_mile" binding:"required"`
	Address    string  `json:"address,omitempty"`
}

// SetSessionLocationHandler sets or updates the location for an existing session
func (s *Server) SetSessionLocationHandler(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	// Get the Cognito user ID from context
	cognitoUserID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Look up the database user ID based on Cognito user ID
	databaseUserID, err := auth.LookupDatabaseUserID(r.Context(), s.db, cognitoUserID)
	if err != nil {
		log.Printf("❌ [SET LOCATION] Failed to lookup database user ID: %v", err)
		http.Error(w, "User not found in database", http.StatusNotFound)
		return
	}

	// Validate that the session exists and belongs to the user
	queries := db.New(s.db)
	session, err := queries.GetSessionByID(r.Context(), sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if session.UserID != databaseUserID {
		http.Error(w, "Session does not belong to user", http.StatusForbidden)
		return
	}

	// Parse request body
	var req SetLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.Latitude < -90 || req.Latitude > 90 {
		http.Error(w, "Invalid latitude. Must be between -90 and 90", http.StatusBadRequest)
		return
	}
	if req.Longitude < -180 || req.Longitude > 180 {
		http.Error(w, "Invalid longitude. Must be between -180 and 180", http.StatusBadRequest)
		return
	}
	if req.RadiusMile <= 0 || req.RadiusMile > 310 {
		http.Error(w, "Invalid radius. Must be between 0 and 310 miles", http.StatusBadRequest)
		return
	}

	// Create location object
	location := &SessionLocation{
		Latitude:   req.Latitude,
		Longitude:  req.Longitude,
		RadiusMile: req.RadiusMile,
		Address:    req.Address,
	}

	// Update session location
	err = UpdateSessionLocation(r.Context(), s.db, sessionID, location)
	if err != nil {
		log.Printf("ERROR: Failed to update session location: %v", err)
		http.Error(w, "Failed to update session location", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ [SET LOCATION] Location updated for session %s: lat=%.6f, lng=%.6f, radius=%.1f miles",
		sessionID, req.Latitude, req.Longitude, req.RadiusMile)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Location updated successfully",
		"location": map[string]interface{}{
			"latitude":    req.Latitude,
			"longitude":   req.Longitude,
			"radius_mile": req.RadiusMile,
			"address":     req.Address,
			"set_at":      time.Now().Format(time.RFC3339),
		},
	})
}

// GetDatabaseUserIDHandler returns the database user ID for the authenticated user
func (s *Server) GetDatabaseUserIDHandler(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Get the Cognito user ID from context
	cognitoUserID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Look up the database user ID based on Cognito user ID
	databaseUserID, err := auth.LookupDatabaseUserID(r.Context(), s.db, cognitoUserID)
	if err != nil {
		log.Printf("❌ [GET DB USER ID] Failed to lookup database user ID: %v", err)
		http.Error(w, "User not found in database", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"database_user_id": databaseUserID,
		"cognito_user_id":  cognitoUserID,
	})
}
