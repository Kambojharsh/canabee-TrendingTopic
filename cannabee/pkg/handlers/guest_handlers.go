package handlers

import (
	"cannabee/pkg/auth"
	"cannabee/pkg/db"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// GuestSessionResponse represents the response for guest session operations
type GuestSessionResponse struct {
	SessionID        string `json:"session_id"`
	GuestID          string `json:"guest_id"`
	SessionStartTime string `json:"session_start_time"`
	LastAccessedAt   string `json:"last_accessed_at"`
	IsActive         bool   `json:"is_active"`
	Metadata         string `json:"metadata"`
}

// CreateGuestSessionRequest represents the request to create a guest session
type CreateGuestSessionRequest struct {
	GuestID      string      `json:"guest_id"`      // UUID provided by frontend
	AccessKey    string      `json:"access_key"`    // AWS temporary credentials
	SecretKey    string      `json:"secret_key"`    // AWS temporary credentials
	SessionToken string      `json:"session_token"` // AWS temporary credentials
	Metadata     interface{} `json:"metadata,omitempty"`
}

// CreateGuestSessionResponse represents the response after creating a guest session
type CreateGuestSessionResponse struct {
	SessionID string `json:"session_id"`
	GuestID   string `json:"guest_id"`
	Metadata  string `json:"metadata"`
	IsNewUser bool   `json:"is_new_user"` // Indicates if this is a newly created guest user
}

// GetGuestSessionsHandler retrieves all sessions for a guest user
// NOTE: Guest sessions are deleted when closed, so this will only return active sessions
func (s *Server) GetGuestSessionsHandler(w http.ResponseWriter, r *http.Request) {
        log.Printf("GetGuestSessionsHandler")
	guestID := r.URL.Query().Get("guest_id")
	if guestID == "" {
		http.Error(w, "guest_id parameter required", http.StatusBadRequest)
		return
	}

	// Validate AWS credentials from headers
	accessKey := r.Header.Get("X-AWS-Access-Key")
	secretKey := r.Header.Get("X-AWS-Secret-Key")
	sessionToken := r.Header.Get("X-AWS-Session-Token")

	if accessKey == "" || secretKey == "" || sessionToken == "" {
		http.Error(w, "AWS credentials required in headers", http.StatusUnauthorized)
		return
	}

	// Validate guest credentials
	_, err := auth.ValidateGuestCredentials(context.Background(), accessKey, secretKey, sessionToken)
	if err != nil {
		log.Printf("Guest credential validation failed: %v", err)
		http.Error(w, "Invalid or expired credentials", http.StatusUnauthorized)
		return
	}

	queries := db.New(s.db)
	sessions, err := queries.GetGuestSessionsByGuestID(context.Background(), guestID)
	if err != nil {
		http.Error(w, "Failed to get sessions", http.StatusInternalServerError)
		return
	}

	var response []GuestSessionResponse
	for _, session := range sessions {
		// Only return active sessions (closed ones are deleted)
		if !session.IsActive {
			continue
		}

		sessionResp := GuestSessionResponse{
			SessionID:        session.SessionID,
			GuestID:          session.GuestID,
			IsActive:         session.IsActive,
			SessionStartTime: session.SessionStartTime.Format("2006-01-02T15:04:05Z"),
			LastAccessedAt:   session.LastAccessedAt.Format("2006-01-02T15:04:05Z"),
		}

		if session.Metadata.Valid {
			sessionResp.Metadata = session.Metadata.String
		}

		response = append(response, sessionResp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CreateGuestSessionHandler creates a new session for a guest user
func (s *Server) CreateGuestSessionHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateGuestSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.GuestID == "" {
		http.Error(w, "guest_id is required", http.StatusBadRequest)
		return
	}

	if req.AccessKey == "" || req.SecretKey == "" || req.SessionToken == "" {
		http.Error(w, "AWS credentials (access_key, secret_key, session_token) are required", http.StatusBadRequest)
		return
	}

	log.Printf("=== Creating Guest Session ===")
	log.Printf("Guest ID: %s", req.GuestID)

	// Validate guest credentials with AWS STS
	validationResp, err := auth.ValidateGuestCredentials(context.Background(), req.AccessKey, req.SecretKey, req.SessionToken)
	if err != nil {
		log.Printf("Guest credential validation failed: %v", err)
		http.Error(w, "Invalid or expired AWS credentials", http.StatusUnauthorized)
		return
	}

	log.Printf("✅ Guest credentials validated")
	log.Printf("AWS Account: %s", validationResp.Account)
	log.Printf("Cognito Identity ID: %s", validationResp.IdentityID)

	queries := db.New(s.db)
	ctx := context.Background()

	// Check if guest user already exists
	existingGuest, err := queries.GetGuestUserByID(ctx, req.GuestID)
	isNewUser := false

	if err != nil {
		if err == sql.ErrNoRows {
			// Guest user doesn't exist, create new one
			log.Printf("Creating new guest user with ID: %s", req.GuestID)

			// TODO: Insert guest user into database
			err = queries.CreateGuestUser(ctx, db.CreateGuestUserParams{
				GuestID: req.GuestID,
				CognitoIdentityID: sql.NullString{
					String: validationResp.IdentityID,
					Valid:  validationResp.IdentityID != "",
				},
				AwsAccount: sql.NullString{
					String: validationResp.Account,
					Valid:  validationResp.Account != "",
				},
				AwsUserID: sql.NullString{
					String: validationResp.UserId,
					Valid:  validationResp.UserId != "",
				},
				Metadata: sql.NullString{
					String: "{}",
					Valid:  true,
				},
			})
			if err != nil {
				log.Printf("Error creating guest user: %v", err)
				http.Error(w, "Failed to create guest user", http.StatusInternalServerError)
				return
			}
			isNewUser = true
			log.Printf("✅ Guest user created successfully")
		} else {
			log.Printf("Error checking guest user: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	} else {
		log.Printf("Guest user already exists: %s", existingGuest.GuestID)
		// Update last accessed time
		queries.UpdateGuestUserAccess(ctx, req.GuestID)
	}

	// Generate UUID for session
	sessionID := uuid.New().String()
	now := time.Now()

	// Handle metadata as JSON
	var metadataJSON string
	var metadataValid bool

	if req.Metadata != nil {
		metadataBytes, err := json.Marshal(req.Metadata)
		if err != nil {
			http.Error(w, "Invalid metadata format", http.StatusBadRequest)
			return
		}
		metadataJSON = string(metadataBytes)
		metadataValid = true
	} else {
		metadataJSON = "{}"
		metadataValid = true
	}

	// TODO: Insert guest session into database
	err = queries.CreateGuestSession(ctx, db.CreateGuestSessionParams{
		SessionID:        sessionID,
		GuestID:          req.GuestID,
		SessionStartTime: now,
		LastAccessedAt:   now,
		IsActive:         true,
		Metadata:         sql.NullString{String: metadataJSON, Valid: metadataValid},
		Summary:          sql.NullString{Valid: false},
	})
	if err != nil {
		log.Printf("Error creating guest session: %v", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Guest session created: %s", sessionID)

	response := CreateGuestSessionResponse{
		SessionID: sessionID,
		GuestID:   req.GuestID,
		Metadata:  metadataJSON,
		IsNewUser: isNewUser,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// GetGuestSessionHandler retrieves a specific guest session
func (s *Server) GetGuestSessionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	// Validate AWS credentials from headers
	accessKey := r.Header.Get("X-AWS-Access-Key")
	secretKey := r.Header.Get("X-AWS-Secret-Key")
	sessionToken := r.Header.Get("X-AWS-Session-Token")

	if accessKey == "" || secretKey == "" || sessionToken == "" {
		http.Error(w, "AWS credentials required in headers", http.StatusUnauthorized)
		return
	}

	// Validate guest credentials
	_, err := auth.ValidateGuestCredentials(context.Background(), accessKey, secretKey, sessionToken)
	if err != nil {
		log.Printf("Guest credential validation failed: %v", err)
		http.Error(w, "Invalid or expired credentials", http.StatusUnauthorized)
		return
	}

	queries := db.New(s.db)
	session, err := queries.GetGuestSessionByID(context.Background(), sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	response := GuestSessionResponse{
		SessionID:        session.SessionID,
		GuestID:          session.GuestID,
		IsActive:         session.IsActive,
		SessionStartTime: session.SessionStartTime.Format("2006-01-02T15:04:05Z"),
		LastAccessedAt:   session.LastAccessedAt.Format("2006-01-02T15:04:05Z"),
	}

	if session.Metadata.Valid {
		response.Metadata = session.Metadata.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DeleteGuestSessionHandler deletes a specific guest session
func (s *Server) DeleteGuestSessionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	// Validate AWS credentials from headers
	accessKey := r.Header.Get("X-AWS-Access-Key")
	secretKey := r.Header.Get("X-AWS-Secret-Key")
	sessionToken := r.Header.Get("X-AWS-Session-Token")

	if accessKey == "" || secretKey == "" || sessionToken == "" {
		http.Error(w, "AWS credentials required in headers", http.StatusUnauthorized)
		return
	}

	// Validate guest credentials
	_, err := auth.ValidateGuestCredentials(context.Background(), accessKey, secretKey, sessionToken)
	if err != nil {
		log.Printf("Guest credential validation failed: %v", err)
		http.Error(w, "Invalid or expired credentials", http.StatusUnauthorized)
		return
	}

	queries := db.New(s.db)
	err = queries.DeleteGuestSession(context.Background(), sessionID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GuestWebSocketHandler handles WebSocket connections for guest users
func (s *Server) GuestWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	guestID := r.URL.Query().Get("guest_id")
	if guestID == "" {
		http.Error(w, "guest_id parameter required", http.StatusBadRequest)
		return
	}

	// Validate AWS credentials from query parameters (for WebSocket, headers aren't always reliable)
	accessKey := r.URL.Query().Get("access_key")
	secretKey := r.URL.Query().Get("secret_key")
	sessionToken := r.URL.Query().Get("session_token")

	if accessKey == "" || secretKey == "" || sessionToken == "" {
		http.Error(w, "AWS credentials required in query parameters", http.StatusUnauthorized)
		return
	}

	log.Printf("=== Guest WebSocket Connection ===")
	log.Printf("Session ID: %s, Guest ID: %s", sessionID, guestID)

	// Validate guest credentials
	validationResp, err := auth.ValidateGuestCredentials(context.Background(), accessKey, secretKey, sessionToken)
	if err != nil {
		log.Printf("Guest credential validation failed: %v", err)
		http.Error(w, "Invalid or expired AWS credentials", http.StatusUnauthorized)
		return
	}

	log.Printf("✅ Guest credentials validated for WebSocket")
	log.Printf("AWS Account: %s", validationResp.Account)

	// Verify that the session exists and belongs to the guest
	queries := db.New(s.db)
	session, err := queries.GetGuestSessionByID(context.Background(), sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if session.GuestID != guestID {
		http.Error(w, "Session does not belong to guest", http.StatusForbidden)
		return
	}

	// Update session access
	queries.UpdateGuestSessionAccess(context.Background(), db.UpdateGuestSessionAccessParams{
		IsActive:  true,
		SessionID: sessionID,
	})

	log.Printf("✅ Guest session verified, upgrading to WebSocket")

	// Use the same WebSocket handler but with guest ID as user ID and guest flag set
	// This allows guests to use the same chat infrastructure but without context/history
	s.wsHub.HandleWebSocketWithGuestFlag(w, r, guestID, sessionID, true)
}

// GetGuestChatMessagesHandler retrieves all messages for a specific guest chat session
// NOTE: This only works for active sessions - closed sessions are deleted
func (s *Server) GetGuestChatMessagesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	guestID := r.URL.Query().Get("guest_id")

	if sessionID == "" {
		http.Error(w, "sessionId parameter required", http.StatusBadRequest)
		return
	}

	if guestID == "" {
		http.Error(w, "guest_id parameter required", http.StatusBadRequest)
		return
	}

	// Validate AWS credentials from headers
	accessKey := r.Header.Get("X-AWS-Access-Key")
	secretKey := r.Header.Get("X-AWS-Secret-Key")
	sessionToken := r.Header.Get("X-AWS-Session-Token")

	if accessKey == "" || secretKey == "" || sessionToken == "" {
		http.Error(w, "AWS credentials required in headers", http.StatusUnauthorized)
		return
	}

	// Validate guest credentials
	_, err := auth.ValidateGuestCredentials(context.Background(), accessKey, secretKey, sessionToken)
	if err != nil {
		log.Printf("Guest credential validation failed: %v", err)
		http.Error(w, "Invalid or expired credentials", http.StatusUnauthorized)
		return
	}

	// Validate session exists and belongs to guest
	queries := db.New(s.db)
	session, err := queries.GetGuestSessionByID(context.Background(), sessionID)
	if err != nil {
		log.Printf("Error getting guest session: %v", err)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	if session.GuestID != guestID {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get messages for the session (using the same chats table)
	chats, err := queries.GetChatsBySessionID(context.Background(), sessionID)
	if err != nil {
		log.Printf("Error getting chats: %v", err)
		http.Error(w, "Failed to get messages", http.StatusInternalServerError)
		return
	}

	// Convert to response format (same as regular users)
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

		// Check if this is a stored product recommendation
		if !chat.IsUserMessage && strings.HasPrefix(strings.TrimSpace(chat.Message.String), "{") && strings.Contains(chat.Message.String, "product_recommendations") {
			var storageMsg struct {
				Type            string      `json:"type"`
				Recommendations interface{} `json:"recommendations"`
				Timestamp       string      `json:"timestamp"`
			}

			if err := json.Unmarshal([]byte(chat.Message.String), &storageMsg); err == nil {
				messages = append(messages, MessageResponse{
					ID:              chat.ID,
					Content:         "Here are some products I found for you:",
					UserType:        userType,
					Timestamp:       timestamp,
					Recommendations: storageMsg.Recommendations,
				})
				continue
			}
		}

		// Regular text message
		messages = append(messages, MessageResponse{
			ID:        chat.ID,
			Content:   chat.Message.String,
			UserType:  userType,
			Timestamp: timestamp,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// UpdateGuestSessionLocation updates the location in a guest session's metadata
func UpdateGuestSessionLocation(ctx context.Context, database *sql.DB, sessionID string, location *SessionLocation) error {
	queries := db.New(database)
	guestSession, err := queries.GetGuestSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}

	var metadata SessionMetadata
	if guestSession.Metadata.Valid {
		json.Unmarshal([]byte(guestSession.Metadata.String), &metadata)
	}

	// Update location
	location.SetAt = time.Now().Format(time.RFC3339)
	metadata.UserLocation = location

	// Convert metadata back to JSON
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	// Update guest session metadata in database
	err = queries.UpdateGuestSessionMetadata(ctx, db.UpdateGuestSessionMetadataParams{
		SessionID: sessionID,
		Metadata:  sql.NullString{String: string(metadataBytes), Valid: true},
	})

	return err
}

// SetGuestSessionLocationHandler sets or updates the location for an existing guest session
func (s *Server) SetGuestSessionLocationHandler(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Handle OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	// Get AWS credentials from headers
	accessKey := r.Header.Get("X-AWS-Access-Key-Id")
	secretKey := r.Header.Get("X-AWS-Secret-Access-Key")
	sessionToken := r.Header.Get("X-AWS-Session-Token")

	if accessKey == "" || secretKey == "" || sessionToken == "" {
		log.Printf("❌ [SET GUEST LOCATION] Missing AWS credentials in headers")
		http.Error(w, "Missing AWS credentials", http.StatusUnauthorized)
		return
	}

	// Validate AWS credentials
	validationResp, err := auth.ValidateGuestCredentials(r.Context(), accessKey, secretKey, sessionToken)
	if err != nil {
		log.Printf("❌ [SET GUEST LOCATION] Invalid credentials: %v", err)
		http.Error(w, "Invalid or expired credentials", http.StatusUnauthorized)
		return
	}

	log.Printf("✅ [SET GUEST LOCATION] Guest credentials validated")
	log.Printf("AWS Account: %s", validationResp.Account)

	// Verify that the session exists
	queries := db.New(s.db)
	guestSession, err := queries.GetGuestSessionByID(r.Context(), sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Guest session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Verify the session is active
	if !guestSession.IsActive {
		http.Error(w, "Guest session is not active", http.StatusForbidden)
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

	// Update guest session location
	err = UpdateGuestSessionLocation(r.Context(), s.db, sessionID, location)
	if err != nil {
		log.Printf("ERROR: Failed to update guest session location: %v", err)
		http.Error(w, "Failed to update guest session location", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ [SET GUEST LOCATION] Location updated for guest session %s: lat=%.6f, lng=%.6f, radius=%.1f miles",
		sessionID, req.Latitude, req.Longitude, req.RadiusMile)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Guest session location updated successfully",
		"location": map[string]interface{}{
			"latitude":    req.Latitude,
			"longitude":   req.Longitude,
			"radius_mile": req.RadiusMile,
			"address":     req.Address,
			"set_at":      time.Now().Format(time.RFC3339),
		},
	})
}
