package websocket

import (
	"cannabee/pkg/chat"
	"cannabee/pkg/constants"
	"cannabee/pkg/db"
	"cannabee/pkg/products"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request, userID string, sessionID string) {
	h.HandleWebSocketWithGuestFlag(w, r, userID, sessionID, false)
}

func (h *Hub) HandleWebSocketWithGuestFlag(w http.ResponseWriter, r *http.Request, userID string, sessionID string, isGuest bool) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:       h,
		conn:      conn,
		send:      make(chan []byte, 256),
		userID:    userID,
		sessionID: sessionID,
		closed:    false,
		mu:        sync.RWMutex{},
		isGuest:   isGuest,
	}

	client.hub.register <- client

	// log the url
	log.Printf("URL: %v", r.URL)

	// Check for skip_greeting parameter
	skipGreeting := r.URL.Query().Get("skip_greeting") == "true"
	log.Printf("Skip greeting parameter: %v", skipGreeting)

	// Check for initial_message parameter
	initialMessage := r.URL.Query().Get("initial_message")
	log.Printf("Initial message parameter: %v", initialMessage != "")
	if initialMessage != "" {
		log.Printf("Initial message content: %.100s...", initialMessage)
	}

	// Check if session is empty before sending greeting
	queries := db.New(h.db)
	messageCount, err := queries.CountChatsBySessionID(context.Background(), sessionID)
	if err != nil {
		log.Printf("Error counting messages: %v", err)
		// Continue anyway, better to show duplicate greeting than fail
	}

	// Get session type to check if it's ask_bee
	sessionType := client.getSessionType()

	var greeting string

	// Only send greeting if this is the first connection to an empty session AND skip_greeting is not set AND it's not ask_bee
	if messageCount == 0 && !skipGreeting && sessionType != "ask_bee" {
		log.Printf("Generating greeting for new session %s, user %s", sessionID, userID)

		// Skip personalized greetings for guests - they have no history
		if isGuest {
			log.Printf("Using default greeting for guest user %s (no context/history)", userID)
			greeting = chat.GetInitialGreeting() // Always use default for guests
		} else {
		// Get user's conversation history context for personalized greeting
		historyContext, err := h.contextService.GetUserSessionContext(context.Background(), userID)
		if err != nil {
			log.Printf("Error getting user history context for %s: %v", userID, err)
			historyContext = &chat.UserSessionContext{SessionCount: 0}
		}

		log.Printf("User %s has %d previous sessions with %d summaries and %d topics",
			userID, historyContext.SessionCount, len(historyContext.RecentSummaries), len(historyContext.RecentTags))

		// Generate personalized greeting based on user's conversation history
		if historyContext.SessionCount > 0 {
			log.Printf("Generating AI-personalized greeting with history for returning user %s", userID)
		} else {
			log.Printf("Using default greeting for new user %s (no history)", userID)
		}

		// Get session type for personalized greeting
		sessionType := client.getSessionType()
		log.Printf("Session type for greeting: %s", sessionType)

		// For "guide_me" sessions, do not use journey history for greeting
		if sessionType == "guide_me" {
			log.Printf("Using LLM-generated session-type-specific greeting for Guide Me session (no journey history)")
			generatedGreeting, err := h.contextService.GenerateSessionTypeGreeting(context.Background(), sessionType)
			if err != nil {
				log.Printf("Error generating session-type greeting for %s: %v, using default", userID, err)
				greeting = chat.GetInitialGreeting()
			} else {
				greeting = generatedGreeting
			}
		} else {
			// Get user config for my_journey sessions
			var userConfig map[string]interface{}
			if sessionType == "my_journey" {
				config, err := client.getUserConfig()
				if err != nil {
					log.Printf("Warning: Failed to get user config for greeting: %v", err)
				} else {
					userConfig = config
					if userConfig != nil {
						log.Printf("Using user config for my_journey greeting: %d preferences", len(userConfig))
					}
				}
			}

			generatedGreeting, err := h.contextService.GeneratePersonalizedGreeting(context.Background(), historyContext, sessionType, userConfig)
			if err != nil {
				log.Printf("Error generating personalized greeting for %s: %v, using default", userID, err)
				greeting = chat.GetInitialGreeting() // Fallback to default
			} else {
				greeting = generatedGreeting
				log.Printf("Successfully generated greeting for user %s: %.100s...", userID, greeting)
			}
		}
		}

		greetingMsg := Message{
			Type:      "message",
			Content:   greeting,
			SessionID: sessionID,
			UserID:    "ai", // "ai" indicates AI message
			Timestamp: time.Now().Format(time.RFC3339),
		}

		// Store greeting in database
		err = queries.CreateChat(context.Background(), db.CreateChatParams{
			SessionID:     sessionID,
			Message:       sql.NullString{String: greeting, Valid: true},
			IsUserMessage: false,
		})
		if err != nil {
			log.Printf("Error storing greeting: %v", err)
		}

		greetingBytes, _ := json.Marshal(greetingMsg)

		if !client.safeSend(greetingBytes) {
			log.Printf("Unable to send greeting message, client likely disconnected")
		}

		// Lazily cleanup old session tags for this user (skip for guests)
		if !isGuest {
		go func() {
			if err := h.contextService.CleanupOldUserTags(context.Background(), userID); err != nil {
				log.Printf("Error cleaning up old tags for user %s: %v", userID, err)
			}
		}()
		}
	} else {
		if skipGreeting {
			log.Printf("Skipping greeting for session %s due to skip_greeting parameter", sessionID)
		} else if sessionType == "ask_bee" {
			log.Printf("Skipping greeting for ask_bee session %s", sessionID)
		} else {
			// Session has existing messages - frontend will load them via HTTP
			log.Printf("Session %s has existing messages, frontend will load via HTTP", sessionID)
		}
	}

	// Send initial message if provided
	if initialMessage != "" {
		log.Printf("Sending initial message for session %s: %.100s...", sessionID, initialMessage)

		// Create initial message
		initialMsg := Message{
			Type:      "message",
			Content:   initialMessage,
			SessionID: sessionID,
			UserID:    userID,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		// Store initial message in database
		err = queries.CreateChat(context.Background(), db.CreateChatParams{
			SessionID:     sessionID,
			Message:       sql.NullString{String: initialMessage, Valid: true},
			IsUserMessage: true,
		})
		if err != nil {
			log.Printf("Error storing initial message: %v", err)
		}

		// Send initial message to client
		initialMsgBytes, _ := json.Marshal(initialMsg)
		if !client.safeSend(initialMsgBytes) {
			log.Printf("Unable to send initial message, client likely disconnected")
		} else {
			log.Printf("Successfully sent initial message for session %s", sessionID)

			// Debug: Check connection status right after sending initial message
			client.mu.RLock()
			isClosed := client.closed
			client.mu.RUnlock()
			log.Printf("DEBUG: Connection status after sending initial message - closed: %v", isClosed)

			// Process the initial message to get AI response
			go func() {
				// Longer delay to ensure WebSocket connection is fully stable after upgrade
				time.Sleep(1000 * time.Millisecond) // Increased from 100ms to 1000ms

				// Debug: Check connection status before processing
				client.mu.RLock()
				isClosed := client.closed
				client.mu.RUnlock()
				log.Printf("DEBUG: Initial message processing - connection closed: %v", isClosed)

				// Get chat history for context
				chatHistory, err := client.getChatHistory()
				if err != nil {
					log.Printf("Error getting chat history for initial message response: %v", err)
					return
				}

				// Get user's conversation history context for enhanced AI responses
				// Skip context for guests
				var historyContext *chat.UserSessionContext
				if isGuest {
					log.Printf("Guest user %s - skipping conversation history for initial message", userID)
					historyContext = &chat.UserSessionContext{SessionCount: 0}
				} else {
					historyContext, err = h.contextService.GetUserSessionContext(context.Background(), userID)
				if err != nil {
					log.Printf("Error getting user history context for initial message response: %v", err)
					historyContext = &chat.UserSessionContext{SessionCount: 0}
				}
				}

				log.Printf("Processing initial message with AI response for session %s", sessionID)

				// Get AI response and stream it
				client.streamAIResponseWithHistoryContext(chatHistory, historyContext, client.isGuest)
			}()
		}
	}

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		log.Printf("DEBUG: readPump exiting for session %s (guest: %v)", c.sessionID, c.isGuest)

		// Only complete session for authenticated users, not guests
		if !c.isGuest {
		go c.completeSession()
		} else {
			// For guests, delete the session entirely
			log.Printf("Guest session %s closing - will be deleted", c.sessionID)
			// go c.deleteGuestSession()
		}

		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(constants.WSDefaultTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(constants.WSDefaultTimeout))
		return nil
	})

	log.Printf("DEBUG: readPump started for session %s", c.sessionID)

	for {
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			log.Printf("DEBUG: readPump exiting due to error for session %s: %v", c.sessionID, err)
			break
		}

		var msg Message
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		// Validate message content
		if strings.TrimSpace(msg.Content) == "" {
			log.Printf("Warning: Received empty message from user %s, ignoring", c.userID)
			continue
		}

		// Store user message in database
		queries := db.New(c.hub.db)
		err = queries.CreateChat(context.Background(), db.CreateChatParams{
			SessionID:     c.sessionID,
			Message:       sql.NullString{String: msg.Content, Valid: true},
			IsUserMessage: true,
		})
		if err != nil {
			log.Printf("Error storing user message: %v", err)
		}

		// Note: User message confirmation removed to prevent duplicate messages
		// Frontend displays user messages immediately when sent

		// Get chat history for context
		chatHistory, err := c.getChatHistory()
		if err != nil {
			log.Printf("Error getting chat history: %v", err)
			continue
		}

		// Get user's conversation history context for enhanced AI responses
		// Skip context for guests - they have no history
		var historyContext *chat.UserSessionContext
		if c.isGuest {
			log.Printf("Guest user %s - skipping conversation history (no context)", c.userID)
			historyContext = &chat.UserSessionContext{SessionCount: 0}
		} else {
			historyContext, err = c.hub.contextService.GetUserSessionContext(context.Background(), c.userID)
		if err != nil {
			log.Printf("Error getting user history context for AI response: %v", err)
			historyContext = &chat.UserSessionContext{SessionCount: 0}
		}

		log.Printf("Enhancing AI response with conversation history - User %s: %d sessions, %d recent topics",
			c.userID, historyContext.SessionCount, len(historyContext.RecentTags))
		}

		// Get AI response and stream it with full conversation history context for personalized experience
		go c.streamAIResponseWithHistoryContext(chatHistory, historyContext, c.isGuest)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// safeSend sends a message to the client's send channel without panicking on closed channel
func (c *Client) safeSend(data []byte) bool {
	log.Printf("DEBUG: safeSend called - data size: %d bytes, sessionID: %s, userID: %s", len(data), c.sessionID, c.userID)

	c.mu.RLock()
	isClosed := c.closed
	c.mu.RUnlock()

	if isClosed {
		log.Printf("DEBUG: safeSend failed - client is closed")
		return false
	}

	select {
	case c.send <- data:
		return true
	default:
		// Channel is closed or full, client likely disconnected
		log.Printf("DEBUG: safeSend failed - channel is full or closed")
		return false
	}
}

// safeClose closes the client's send channel safely, preventing double-close panics
func (c *Client) safeClose() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.closed {
		c.closed = true
		close(c.send)
		log.Printf("DEBUG: Client connection marked as closed for session %s", c.sessionID)
	}
}

func (c *Client) getChatHistory() ([]chat.ChatMessage, error) {
	queries := db.New(c.hub.db)
	chats, err := queries.GetChatsBySessionID(context.Background(), c.sessionID)
	if err != nil {
		return nil, err
	}

	var messages []chat.ChatMessage
	for _, chatRecord := range chats {
		role := "assistant"
		if chatRecord.IsUserMessage {
			role = "user"
		}
		messages = append(messages, chat.ChatMessage{
			Role:    role,
			Content: chatRecord.Message.String,
		})
	}

	return messages, nil
}

// getUserConfig retrieves the user's configuration from the database
func (c *Client) getUserConfig() (map[string]interface{}, error) {
	queries := db.New(c.hub.db)
	user, err := queries.GetUserByID(context.Background(), c.userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Parse the config JSON
	var config map[string]interface{}
	if user.Config != nil && len(user.Config) > 0 {
		if err := json.Unmarshal(user.Config, &config); err != nil {
			log.Printf("Warning: Failed to parse user config JSON: %v", err)
			return nil, nil
		}
	}

	return config, nil
}

// getSessionType retrieves the session type from the session metadata
func (c *Client) getSessionType() string {
	queries := db.New(c.hub.db)
	session, err := queries.GetSessionByID(context.Background(), c.sessionID)
	if err != nil {
		log.Printf("Warning: Failed to get session for type detection: %v", err)
		return ""
	}

	if !session.Metadata.Valid || session.Metadata.String == "" {
		return ""
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(session.Metadata.String), &metadata); err != nil {
		log.Printf("Warning: Failed to parse session metadata: %v", err)
		return ""
	}

	if sessionType, ok := metadata["session_type"].(string); ok {
		return sessionType
	}

	return ""
}

// getSessionConfig retrieves the session-specific config from the session metadata
func (c *Client) getSessionConfig() map[string]interface{} {
	queries := db.New(c.hub.db)
	session, err := queries.GetSessionByID(context.Background(), c.sessionID)
	if err != nil {
		log.Printf("Warning: Failed to get session for config detection: %v", err)
		return nil
	}

	if !session.Metadata.Valid || session.Metadata.String == "" {
		return nil
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(session.Metadata.String), &metadata); err != nil {
		log.Printf("Warning: Failed to parse session metadata: %v", err)
		return nil
	}

	if sessionConfig, ok := metadata["session_config"].(map[string]interface{}); ok {
		return sessionConfig
	}

	return nil
}

// createSystemPromptWithUserConfig creates a system prompt that includes the user's configuration and session type
func (c *Client) createSystemPromptWithUserConfig() string {
	// Get session-specific config first (takes priority)
	sessionConfig := c.getSessionConfig()

	// Get user's global config as fallback
	userConfig, err := c.getUserConfig()
	if err != nil {
		log.Printf("Warning: Failed to get user config for system prompt: %v", err)
	}

	// Use session config if available, otherwise use user config
	var config map[string]interface{}
	var configSource string
	if sessionConfig != nil {
		config = sessionConfig
		configSource = "session-specific"
	} else if userConfig != nil {
		config = userConfig
		configSource = "user profile"
	}

	// Get session type from metadata
	sessionType := c.getSessionType()

	// Create enhanced system prompt with user config and session type
	basePrompt := chat.GetMainConversationalSystemPrompt()

	var enhancedPrompt string

	switch sessionType {
	case "my_journey":
		// For "My Journey" sessions, emphasize personal exploration
		enhancedPrompt = basePrompt + "\n\nSESSION TYPE: My Journey - Personal Cannabis Exploration\n"
		enhancedPrompt += "This is a personal exploration session where the user wants to explore cannabis on their own terms. "
		enhancedPrompt += "Focus on providing educational information, answering questions, and supporting their personal journey. "
		enhancedPrompt += "Be supportive and informative without being overly prescriptive."

		if config != nil {
			enhancedPrompt += fmt.Sprintf("\n\nUSER PREFERENCES (%s):\n", configSource)
			for key, value := range config {
				enhancedPrompt += fmt.Sprintf("- %s: %v\n", key, value)
			}
			enhancedPrompt += "\n\nUse the above user preferences to personalize your responses and recommendations."
		}
	case "guide_me":
		// For "Guide Me" sessions, emphasize personalized guidance
		enhancedPrompt = basePrompt + "\n\nSESSION TYPE: Guide Me - Personalized Recommendations\n"
		enhancedPrompt += "This is a guidance session where the user wants personalized recommendations and advice. "
		enhancedPrompt += "Use their preferences and previous session context to provide tailored suggestions. "
		enhancedPrompt += "Be proactive in offering specific recommendations based on their needs and preferences."
	case "ask_bee":
		enhancedPrompt = basePrompt + "\n\nSESSION TYPE: Ask Bee - Personalized Recommendations\n"
		enhancedPrompt += "This is a session where the user wants to ask questions about cannabis. "
		enhancedPrompt += "Use the input from the user to provide more information specific to cannabis. "
		enhancedPrompt += "Be proactive in offering specific recommendations based on their needs and preferences."
	default:
		// Default behavior for sessions without type
		if config != nil {
			enhancedPrompt = basePrompt + fmt.Sprintf("\n\nUSER PREFERENCES (%s):\n", configSource)
			for key, value := range config {
				enhancedPrompt += fmt.Sprintf("- %s: %v\n", key, value)
			}
			enhancedPrompt += "\n\nUse the above user preferences to personalize your responses and recommendations."
		} else {
			enhancedPrompt = basePrompt
		}
	}

	log.Printf("Enhanced system prompt for session %s (type: %s) with %s config: %t",
		c.sessionID, sessionType, configSource, config != nil)
	log.Printf("Enhanced prompt length: %d characters", len(enhancedPrompt))
	log.Printf("Enhanced prompt preview: %.500s...", enhancedPrompt)

	return enhancedPrompt
}

func (c *Client) streamAIResponseWithHistoryContext(messages []chat.ChatMessage, historyContext *chat.UserSessionContext, isGuest bool) {
	log.Printf("=== Starting AI Response Stream with Enhanced Context ===")
	log.Printf("Session: %s, User: %s, Guest: %v", c.sessionID, c.userID, isGuest)
	log.Printf("Input messages: %d", len(messages))
	log.Printf("History context available: %t", historyContext != nil && historyContext.SessionCount > 0)

	ctx, cancel := context.WithTimeout(context.Background(), constants.WSAIResponseTimeout)
	defer cancel()

	// Get the last user message for knowledge and product recommendation queries
	lastUserMessage := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			lastUserMessage = messages[i].Content
			break
		}
	}
	log.Printf("Last user message for analysis: '%.100s...'", lastUserMessage)

	// Run knowledge retrieval and product recommendation analysis in parallel
	log.Printf("=== Starting Parallel Operations ===")
	var knowledgeResponse *chat.KnowledgeResponse
	var productRecommendations *products.ProductRecommendationResponse
	var suggestionResponse *chat.SuggestionResponse
	var wg sync.WaitGroup

	// Channel for errors
	errorChan := make(chan error, 3)

	// Track parallel operation timing
	startTime := time.Now()

	// Knowledge retrieval
	if c.hub.KnowledgeService != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			knowledgeStart := time.Now()
			log.Printf("[PARALLEL-KNOWLEDGE] Starting knowledge retrieval...")

			knowledgeCtx, knowledgeCancel := context.WithTimeout(context.Background(), constants.WSKnowledgeRetrievalTimeout)
			defer knowledgeCancel()

			knowledge, err := c.hub.KnowledgeService.GetRelevantKnowledge(knowledgeCtx, lastUserMessage)
			if err != nil {
				log.Printf("[PARALLEL-KNOWLEDGE] ERROR: %v", err)
				errorChan <- fmt.Errorf("knowledge retrieval failed: %w", err)
				return
			}
			knowledgeResponse = knowledge
			knowledgeDuration := time.Since(knowledgeStart)
			log.Printf("[PARALLEL-KNOWLEDGE] COMPLETED in %v: relevant=%t, results=%d",
				knowledgeDuration, knowledge.HasRelevantKnowledge, len(knowledge.Results))
		}()
	} else {
		log.Printf("[PARALLEL-KNOWLEDGE] SKIPPED: Cannabis knowledge service not available")
	}

	// Product recommendation analysis
	if c.hub.ProductRecommendationService != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			productStart := time.Now()
			log.Printf("[PARALLEL-PRODUCTS] Starting product recommendation analysis...")
			log.Printf("[PARALLEL-PRODUCTS] Input messages count: %d", len(messages))
			log.Printf("[PARALLEL-PRODUCTS] Last user message: '%.100s...'", lastUserMessage)

			productCtx, productCancel := context.WithTimeout(context.Background(), constants.WSProductRecommendationTimeout)
			defer productCancel()

			// Get user location from session for location-aware recommendations
			userLocation := c.getUserLocationFromSession(productCtx, isGuest)
			if userLocation != nil {
				log.Printf("[PARALLEL-PRODUCTS] Using location filtering: lat=%.6f, lng=%.6f, radius=%.1f miles",
					userLocation.Latitude, userLocation.Longitude, userLocation.RadiusMile)
			} else {
				log.Printf("[PARALLEL-PRODUCTS] No location data available - will be handled by product service")
			}

			// Call product service - it will handle location validation internally
			recommendations, err := c.hub.ProductRecommendationService.GetRecommendationsWithLocation(productCtx, messages, userLocation, isGuest)
			if err != nil {
				log.Printf("[PARALLEL-PRODUCTS] ERROR: %v", err)
				errorChan <- fmt.Errorf("product recommendations failed: %w", err)
				return
			}
			productRecommendations = recommendations
			productDuration := time.Since(productStart)
			log.Printf("[PARALLEL-PRODUCTS] COMPLETED in %v: should_show=%t, products=%d",
				productDuration, recommendations.ShouldShowRecommendations, len(recommendations.Products))
			log.Printf("[PARALLEL-PRODUCTS] Query: '%s'", recommendations.Query)
			log.Printf("[PARALLEL-PRODUCTS] Reason: '%s'", recommendations.Reason)
		}()
	} else {
		log.Printf("[PARALLEL-PRODUCTS] SKIPPED: Product recommendation service not available")
	}

	// Suggestion analysis
	if c.hub.SuggestionService != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			suggestionStart := time.Now()
			log.Printf("[PARALLEL-SUGGESTIONS] Starting suggestion analysis...")

			suggestionCtx, suggestionCancel := context.WithTimeout(context.Background(), constants.WSSuggestionAnalysisTimeout)
			defer suggestionCancel()

			suggestions, err := c.hub.SuggestionService.AnalyzeConversationForSuggestions(suggestionCtx, messages)
			if err != nil {
				log.Printf("[PARALLEL-SUGGESTIONS] ERROR: %v", err)
				errorChan <- fmt.Errorf("suggestion analysis failed: %w", err)
				return
			}
			suggestionResponse = suggestions
			suggestionDuration := time.Since(suggestionStart)
			log.Printf("[PARALLEL-SUGGESTIONS] COMPLETED in %v: should_show=%t, suggestions=%d",
				suggestionDuration, suggestions.ShouldShowSuggestions, len(suggestions.Suggestions))
		}()
	} else {
		log.Printf("[PARALLEL-SUGGESTIONS] SKIPPED: Suggestion service not available")
	}

	// Wait for parallel operations to complete
	log.Printf("Waiting for parallel operations to complete...")
	wg.Wait()
	close(errorChan)

	parallelDuration := time.Since(startTime)
	log.Printf("=== Parallel Operations Complete in %v ===", parallelDuration)

	// Log any errors (but continue with AI response)
	errorCount := 0
	for err := range errorChan {
		errorCount++
		log.Printf("Parallel operation error %d: %v", errorCount, err)
	}

	if errorCount > 0 {
		log.Printf("WARNING: %d parallel operations failed, continuing with available data", errorCount)
	}

	// Enhance messages with conversation history context if available
	// For "guide_me" sessions, do not use journey history
	sessionType := c.getSessionType()
	var enhancedMessages []chat.ChatMessage
	if sessionType == "guide_me" {
		log.Printf("=== Skipping History Context for Guide Me Session ===")
		enhancedMessages = messages
	} else {
		log.Printf("=== Enhancing Messages with History Context ===")
		enhancedMessages = c.enhanceMessagesWithHistoryContext(messages, historyContext)
	}
	log.Printf("Enhanced messages count: %d", len(enhancedMessages))

	// Get AI response using cannabis knowledge context
	log.Printf("=== Creating AI Stream Response ===")

	// Check if we should show product recommendations (even if we don't have products yet due to missing location)
	hasProductRecommendations := productRecommendations != nil && productRecommendations.ShouldShowRecommendations

	// flow summary only
	log.Printf("Product recommendations available: %t", productRecommendations != nil)
	if productRecommendations != nil {
		log.Printf("Should show recommendations: %t", productRecommendations.ShouldShowRecommendations)
		log.Printf("Number of products: %d", len(productRecommendations.Products))
		log.Printf("Query: '%s'", productRecommendations.Query)
		log.Printf("Reason: '%s'", productRecommendations.Reason)
	}
	log.Printf("Will send product recommendations: %t", hasProductRecommendations)

	// Skip AI response if we have product recommendations to avoid duplicate content
	if hasProductRecommendations {
		log.Printf("🚀 SKIPPING AI response - will send product recommendations directly")
	} else {
		// Generate AI response only when no product recommendations are available
		var stream *openai.ChatCompletionStream
		var err error

		// Get enhanced system prompt with user config
		enhancedSystemPrompt := c.createSystemPromptWithUserConfig()

		if knowledgeResponse != nil && knowledgeResponse.HasRelevantKnowledge {
			log.Printf("Using KNOWLEDGE-ENHANCED AI response with user config")
			log.Printf("Knowledge context: %d characters from %d results",
				len(knowledgeResponse.ContextText), len(knowledgeResponse.Results))
			stream, err = c.hub.openai.GetStreamResponseWithKnowledgeAndSystemPrompt(ctx, enhancedMessages, knowledgeResponse, enhancedSystemPrompt)
		} else {
			if knowledgeResponse != nil {
				log.Printf("Using STANDARD AI response with user config (knowledge available but not relevant)")
			} else {
				log.Printf("Using STANDARD AI response with user config (no knowledge service)")
			}
			stream, err = c.hub.openai.GetStreamResponseWithSystemPrompt(ctx, enhancedMessages, enhancedSystemPrompt)
		}

		if err != nil {
			log.Printf("ERROR: Failed to create AI stream: %v", err)
			return
		}
		defer stream.Close()

		log.Printf("AI stream created successfully, starting response transmission...")

		var fullResponse strings.Builder
		chunkCount := 0

		// Start streaming message
		startMsg := Message{
			Type:      "stream_start",
			Content:   "",
			SessionID: c.sessionID,
			UserID:    "ai",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		startBytes, _ := json.Marshal(startMsg)
		if !c.safeSend(startBytes) {
			log.Printf("ERROR: Unable to send stream start, client likely disconnected")
			return
		}
		log.Printf("Stream start message sent")

		// Read and stream chunks
		streamStart := time.Now()
		for {
			response, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					log.Printf("Stream complete after %v and %d chunks", time.Since(streamStart), chunkCount)
					break
				}
				log.Printf("ERROR: Stream receive error: %v", err)
				break
			}

			if len(response.Choices) > 0 && response.Choices[0].Delta.Content != "" {
				chunkCount++
				content := response.Choices[0].Delta.Content
				fullResponse.WriteString(content)

				// Send chunk
				chunkMsg := Message{
					Type:      "stream_chunk",
					Content:   content,
					SessionID: c.sessionID,
					UserID:    "ai",
					Timestamp: time.Now().Format(time.RFC3339),
				}
				chunkBytes, _ := json.Marshal(chunkMsg)
				if !c.safeSend(chunkBytes) {
					log.Printf("ERROR: Unable to send stream chunk, client likely disconnected")
					return
				}
			}
		}

		log.Printf("Stream chunks complete - sent %d chunks, %d characters", chunkCount, fullResponse.Len())

		// End streaming
		endMsg := Message{
			Type:      "stream_end",
			Content:   "",
			SessionID: c.sessionID,
			UserID:    "ai",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		endBytes, _ := json.Marshal(endMsg)
		if !c.safeSend(endBytes) {
			log.Printf("ERROR: Unable to send stream end, client likely disconnected")
			return
		}

		// Store AI response in database
		if fullResponse.Len() > 0 {
			log.Printf("Storing AI response in database (%d characters)...", fullResponse.Len())
			queries := db.New(c.hub.db)
			err = queries.CreateChat(context.Background(), db.CreateChatParams{
				SessionID:     c.sessionID,
				Message:       sql.NullString{String: fullResponse.String(), Valid: true},
				IsUserMessage: false,
			})
			if err != nil {
				log.Printf("ERROR: Failed to store AI response in database: %v", err)
			} else {
				log.Printf("AI response stored in database successfully")
			}
		}
	}

	// Send product recommendations progressively if available
	if hasProductRecommendations {
		log.Printf("🎯 Sending product recommendations progressively...")
		log.Printf("Product recommendations details: %+v", productRecommendations)
		c.sendProductRecommendationsProgressive(productRecommendations)
		log.Printf("✅ Product recommendations sending completed")
	} else {
		log.Printf("❌ No product recommendations to send")
	}

	// Send suggestions if available
	if suggestionResponse != nil && suggestionResponse.ShouldShowSuggestions && len(suggestionResponse.Suggestions) > 0 {
		log.Printf("Sending suggestions...")
		c.sendSuggestions(suggestionResponse)
	}

	totalDuration := time.Since(startTime)
	log.Printf("=== AI Response Stream Complete in %v ===", totalDuration)
}

// sendProductRecommendationsProgressive sends product recommendations progressively to the client
func (c *Client) sendProductRecommendationsProgressive(recommendations *products.ProductRecommendationResponse) {
	log.Printf("=== Sending Product Recommendations Progressively ===")
	log.Printf("Products to send: %d", len(recommendations.Products))
	log.Printf("Recommendation query: '%s'", recommendations.Query)
	log.Printf("Reason: %s", recommendations.Reason)
	log.Printf("Should show recommendations: %t", recommendations.ShouldShowRecommendations)

	// Log each product being sent
	for i, product := range recommendations.Products {
		log.Printf("Product %d: ID=%s, Name='%s', Price=%.2f, Category='%s'",
			i+1, product.ID, product.Name, product.Price, product.Category)
	}

	// If no products (location request), send regular message and return early
	if len(recommendations.Products) == 0 {
		log.Printf("No products - sending location request as regular message")
		locationMsg := Message{
			Type:      "message",
			Content:   recommendations.Reason, // Location request message
			SessionID: c.sessionID,
			UserID:    "ai",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		locationBytes, _ := json.Marshal(locationMsg)
		if !c.safeSend(locationBytes) {
			log.Printf("ERROR: Unable to send location request message, client likely disconnected")
		} else {
			log.Printf("✅ Location request sent as regular message - no product UI triggered")
		}
		return
	}

	// If we have products, send product_stream_intro to trigger product UI
	log.Printf("Products available - sending product_stream_intro")

	// Use custom reason message if provided, otherwise use default
	introContent := "Here are some products I found for you:"
	if recommendations.Reason != "" {
		introContent = recommendations.Reason
		log.Printf("Using custom intro message from recommendations.Reason: '%s'", introContent)
	}

	introMsg := Message{
		Type:      "product_stream_intro",
		Content:   introContent,
		SessionID: c.sessionID,
		UserID:    "ai",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	introBytes, _ := json.Marshal(introMsg)
	if !c.safeSend(introBytes) {
		log.Printf("ERROR: Unable to send product stream intro, client likely disconnected")
		return
	}

	// Send each product individually with a slight delay for progressive effect
	for i, product := range recommendations.Products {
		// Create custom message structure for single product
		type ProductStreamMessage struct {
			Message
			Product      *products.ProductInfo `json:"product"`
			IsLast       bool                  `json:"is_last"`
			TotalCount   int                   `json:"total_count"`
			CurrentIndex int                   `json:"current_index"`
			Reason       string                `json:"reason"`
		}

		productStreamMsg := ProductStreamMessage{
			Message: Message{
				Type:      "product_stream",
				Content:   fmt.Sprintf("Product %d of %d", i+1, len(recommendations.Products)),
				SessionID: c.sessionID,
				UserID:    "ai",
				Timestamp: time.Now().Format(time.RFC3339),
			},
			Product:      &product,
			IsLast:       i == len(recommendations.Products)-1,
			TotalCount:   len(recommendations.Products),
			CurrentIndex: i,
			Reason:       recommendations.Reason,
		}

		msgBytes, err := json.Marshal(productStreamMsg)
		if err != nil {
			log.Printf("ERROR: Failed to marshal product stream message %d: %v", i, err)
			continue
		}

		log.Printf("Sending product stream message %d/%d: %s (%.2f bytes)",
			i+1, len(recommendations.Products), product.Name, float64(len(msgBytes)))

		if !c.safeSend(msgBytes) {
			log.Printf("ERROR: Unable to send product stream message %d, client likely disconnected", i)
			return
		}

		log.Printf("✅ Successfully sent product %d/%d: %s", i+1, len(recommendations.Products), product.Name)
	}

	// Store complete product recommendations in database for persistence
	queries := db.New(c.hub.db)

	// Create a compact JSON representation for storage
	type ProductStorageMessage struct {
		Type            string                                  `json:"type"`
		Recommendations *products.ProductRecommendationResponse `json:"recommendations"`
		Timestamp       string                                  `json:"timestamp"`
	}

	storageMsg := ProductStorageMessage{
		Type:            "product_recommendations",
		Recommendations: recommendations,
		Timestamp:       time.Now().Format(time.RFC3339),
	}

	productDataJSON, err := json.Marshal(storageMsg)
	if err != nil {
		log.Printf("ERROR: Failed to marshal product recommendations for storage: %v", err)
		return
	}

	err = queries.CreateChat(context.Background(), db.CreateChatParams{
		SessionID:     c.sessionID,
		Message:       sql.NullString{String: string(productDataJSON), Valid: true},
		IsUserMessage: false,
	})
	if err != nil {
		log.Printf("ERROR: Failed to store product recommendations in database: %v", err)
	} else {
		log.Printf("Product recommendations stored in database successfully")
	}

	log.Printf("Successfully sent %d products progressively to user %s", len(recommendations.Products), c.userID)
}

// sendSuggestions sends suggestions to the client
func (c *Client) sendSuggestions(suggestionResponse *chat.SuggestionResponse) {
	log.Printf("=== Sending Suggestions ===")
	log.Printf("Suggestions to send: %d", len(suggestionResponse.Suggestions))
	log.Printf("Reason: %s", suggestionResponse.Reason)

	// Create suggestion message
	type SuggestionMessage struct {
		Message
		Suggestions []chat.Suggestion `json:"suggestions"`
	}

	suggestionMsg := SuggestionMessage{
		Message: Message{
			Type:      "suggestions",
			Content:   "Here are some suggestions for you:",
			SessionID: c.sessionID,
			UserID:    "system",
			Timestamp: time.Now().Format(time.RFC3339),
		},
		Suggestions: suggestionResponse.Suggestions,
	}

	msgBytes, err := json.Marshal(suggestionMsg)
	if err != nil {
		log.Printf("ERROR: Failed to marshal suggestions: %v", err)
		return
	}

	if !c.safeSend(msgBytes) {
		log.Printf("ERROR: Unable to send suggestions, client likely disconnected")
		return
	}

	log.Printf("Successfully sent %d suggestions to user %s", len(suggestionResponse.Suggestions), c.userID)
}

// enhanceMessagesWithHistoryContext adds user's conversation history to improve AI responses throughout the session
func (c *Client) enhanceMessagesWithHistoryContext(messages []chat.ChatMessage, historyContext *chat.UserSessionContext) []chat.ChatMessage {
	if historyContext.SessionCount == 0 || len(historyContext.RecentSummaries) == 0 {
		log.Printf("No conversation history available for user %s - using messages as-is", c.userID)
		return messages // No history context to add
	}

	log.Printf("Enhancing AI response with conversation history: %d summaries, %d topics",
		len(historyContext.RecentSummaries), len(historyContext.RecentTags))

	// Create a history context message to prepend
	historyContent := fmt.Sprintf(`Conversation History Context for this user:
Previous discussions: %s
Recent topics discussed: %s

Use this history to provide more personalized and contextually aware responses. Reference previous conversations when relevant, show continuity in care, but always prioritize the current question. Maintain a warm, knowledgeable tone that acknowledges their journey.`,
		strings.Join(historyContext.RecentSummaries, "; "),
		strings.Join(historyContext.RecentTags, ", "))

	historyMessage := chat.ChatMessage{
		Role:    "system",
		Content: historyContent,
	}

	// Prepend history context message to the conversation
	enhancedMessages := make([]chat.ChatMessage, 0, len(messages)+1)
	enhancedMessages = append(enhancedMessages, historyMessage)
	enhancedMessages = append(enhancedMessages, messages...)

	return enhancedMessages
}

// completeSession generates and stores session summary and tags when the session ends
func (c *Client) completeSession() {
	// Only complete sessions that have meaningful conversation (more than just greeting)
	queries := db.New(c.hub.db)
	messageCount, err := queries.CountChatsBySessionID(context.Background(), c.sessionID)
	if err != nil {
		log.Printf("Error counting messages for session completion: %v", err)
		return
	}

	// Need at least 3 messages (greeting + user message + AI response) to be worth summarizing
	if messageCount < 3 {
		log.Printf("Session %s has insufficient messages (%d) for completion processing", c.sessionID, messageCount)
		return
	}

	// Always generate new summary on session completion (replaces old one if exists)
	log.Printf("Starting session completion processing for session %s", c.sessionID)

	// Get chat history for analysis
	chatHistory, err := c.getChatHistory()
	if err != nil {
		log.Printf("Error getting chat history for session completion: %v", err)
		return
	}

	// Filter out empty or system messages for better analysis
	var meaningfulMessages []chat.ChatMessage
	for _, msg := range chatHistory {
		if strings.TrimSpace(msg.Content) != "" && msg.Content != chat.GetInitialGreeting() {
			meaningfulMessages = append(meaningfulMessages, msg)
		}
	}

	if len(meaningfulMessages) < 2 {
		log.Printf("Session %s has insufficient meaningful messages for completion processing", c.sessionID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), constants.WSAIResponseTimeout)
	defer cancel()

	// Generate session summary
	summary, err := c.hub.openai.GenerateSessionSummary(ctx, meaningfulMessages)
	if err != nil {
		log.Printf("Error generating session summary for %s: %v", c.sessionID, err)
	} else {
		// Store/update summary directly in sessions table
		err = queries.UpdateSessionSummary(context.Background(), db.UpdateSessionSummaryParams{
			SessionID: c.sessionID,
			Summary:   sql.NullString{String: summary, Valid: true},
		})
		if err != nil {
			log.Printf("Error storing session summary for %s: %v", c.sessionID, err)
		} else {
			log.Printf("Generated and stored/updated summary for session %s (%.100s...)", c.sessionID, summary)
		}
	}

	// Generate session tags
	tags, err := c.hub.openai.GenerateSessionTags(ctx, meaningfulMessages)
	if err != nil {
		log.Printf("Error generating session tags for %s: %v", c.sessionID, err)
	} else {
		// Get current user's tags and merge with new session tags
		currentUserTags, err := queries.GetUserTags(context.Background(), c.userID)
		if err != nil {
			log.Printf("Error getting current user tags for %s: %v", c.userID, err)
		}

		// Parse current tags from JSON
		var existingTags []string
		if len(currentUserTags) > 0 {
			err = json.Unmarshal(currentUserTags, &existingTags)
			if err != nil {
				log.Printf("Error parsing current user tags for %s: %v", c.userID, err)
				existingTags = []string{}
			}
		}

		// Merge new tags with existing tags (remove duplicates)
		tagSet := make(map[string]bool)
		for _, tag := range existingTags {
			tagSet[tag] = true
		}
		for _, tag := range tags {
			tagSet[tag] = true
		}

		// Convert back to slice
		var mergedTags []string
		for tag := range tagSet {
			mergedTags = append(mergedTags, tag)
		}

		// Update user's tags
		mergedTagsJSON, err := json.Marshal(mergedTags)
		if err != nil {
			log.Printf("Error marshaling merged tags for user %s: %v", c.userID, err)
		} else {
			err = queries.UpdateUserTags(context.Background(), db.UpdateUserTagsParams{
				UserID: c.userID,
				Tags:   json.RawMessage(mergedTagsJSON),
			})
			if err != nil {
				log.Printf("Error updating user tags for %s: %v", c.userID, err)
			} else {
				log.Printf("Generated and merged %d new tags for user %s, total tags: %d", len(tags), c.userID, len(mergedTags))
			}
		}
	}

	// Generate and update session title if needed
	err = c.updateSessionTitleIfNeeded(ctx, meaningfulMessages)
	if err != nil {
		log.Printf("Error updating session title for %s: %v", c.sessionID, err)
	}

	// Mark session as inactive
	err = queries.UpdateSessionStatus(context.Background(), db.UpdateSessionStatusParams{
		SessionID: c.sessionID,
		IsActive:  false,
	})
	if err != nil {
		log.Printf("Error marking session as inactive: %v", err)
	}
}

// updateSessionTitleIfNeeded checks if the session needs a title and generates one if needed
func (c *Client) updateSessionTitleIfNeeded(ctx context.Context, meaningfulMessages []chat.ChatMessage) error {
	queries := db.New(c.hub.db)

	// Get current session metadata
	session, err := queries.GetSessionByID(context.Background(), c.sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session metadata: %w", err)
	}

	// Parse current metadata
	var metadata map[string]interface{}
	if session.Metadata.Valid && session.Metadata.String != "" {
		err = json.Unmarshal([]byte(session.Metadata.String), &metadata)
		if err != nil {
			log.Printf("Error parsing session metadata for %s: %v", c.sessionID, err)
			metadata = make(map[string]interface{})
		}
	} else {
		metadata = make(map[string]interface{})
	}

	// Check if we need to generate a title
	currentTitle, exists := metadata["title"]
	needsTitle := false

	if !exists {
		needsTitle = true
		log.Printf("Session %s has no title, generating one", c.sessionID)
	} else if titleStr, ok := currentTitle.(string); ok {
		// Check if the current title is generic and should be replaced
		genericTitles := []string{"New Session", "Chat Session", "Cannabis Chat Session", "Untitled Session"}
		for _, generic := range genericTitles {
			if titleStr == generic {
				needsTitle = true
				log.Printf("Session %s has generic title '%s', generating meaningful one", c.sessionID, titleStr)
				break
			}
		}
	}

	// Generate title if needed
	if needsTitle {
		title, err := c.hub.openai.GenerateSessionTitle(ctx, meaningfulMessages)
		if err != nil {
			log.Printf("Error generating session title for %s: %v", c.sessionID, err)
			// Don't return error, just log it and continue
		} else {
			// Update metadata with generated title
			metadata["title"] = title

			// Set category based on generated title or use existing one
			if _, hasCategory := metadata["category"]; !hasCategory {
				metadata["category"] = "General"
			}

			// Ensure tags exist
			if _, hasTags := metadata["tags"]; !hasTags {
				metadata["tags"] = []string{"cannabis", "consultation"}
			}

			// Convert metadata back to JSON
			metadataJSON, err := json.Marshal(metadata)
			if err != nil {
				return fmt.Errorf("failed to marshal updated metadata: %w", err)
			}

			// Update session metadata in database
			err = queries.UpdateSessionMetadata(context.Background(), db.UpdateSessionMetadataParams{
				SessionID: c.sessionID,
				Metadata:  sql.NullString{String: string(metadataJSON), Valid: true},
			})
			if err != nil {
				return fmt.Errorf("failed to update session metadata: %w", err)
			}

			log.Printf("Generated and updated session title for %s: '%s'", c.sessionID, title)
		}
	}

	return nil
}

// SessionLocation represents location data stored in session metadata
type SessionLocation struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	RadiusMile float64 `json:"radius_mile"`
	Address    string  `json:"address,omitempty"`
	SetAt      string  `json:"set_at"` // Timestamp when location was set
}

// SessionMetadata represents the structure of session metadata
type SessionMetadata struct {
	SessionType   string                 `json:"session_type,omitempty"`
	SessionConfig map[string]interface{} `json:"session_config,omitempty"`
	UserLocation  *SessionLocation       `json:"user_location,omitempty"`
}

// getUserLocationFromSession extracts user location from session metadata
// For guest users, it fetches from guest_sessions table, otherwise from sessions table
func (c *Client) getUserLocationFromSession(ctx context.Context, isGuest bool) *products.UserLocation {
	if c.sessionID == "" || c.hub.db == nil {
		return nil
	}

	queries := db.New(c.hub.db)
	var metadataString string
	var metadataValid bool

	// Get session metadata based on whether this is a guest or regular user
	if isGuest {
		log.Printf("Fetching location from guest_sessions table for session %s", c.sessionID)
		guestSession, err := queries.GetGuestSessionByID(ctx, c.sessionID)
		if err != nil {
			log.Printf("Could not get guest session %s: %v", c.sessionID, err)
			return nil
		}
		metadataValid = guestSession.Metadata.Valid
		if metadataValid {
			metadataString = guestSession.Metadata.String
		}
	} else {
		log.Printf("Fetching location from sessions table for session %s", c.sessionID)
	session, err := queries.GetSessionByID(ctx, c.sessionID)
	if err != nil {
		log.Printf("Could not get session %s: %v", c.sessionID, err)
		return nil
	}
		metadataValid = session.Metadata.Valid
		if metadataValid {
			metadataString = session.Metadata.String
		}
	}

	if !metadataValid {
		log.Printf("No metadata available for session %s (guest: %v)", c.sessionID, isGuest)
		return nil // No metadata
	}

	// Parse session metadata
	var metadata SessionMetadata
	if err := json.Unmarshal([]byte(metadataString), &metadata); err != nil {
		log.Printf("Could not parse session metadata for %s (guest: %v): %v", c.sessionID, isGuest, err)
		return nil
	}

	// Extract location if available
	if metadata.UserLocation == nil {
		log.Printf("No user location in metadata for session %s (guest: %v)", c.sessionID, isGuest)
		return nil
	}

	log.Printf("Successfully extracted location for session %s (guest: %v): lat=%.6f, lng=%.6f, radius=%.1f miles",
		c.sessionID, isGuest, metadata.UserLocation.Latitude, metadata.UserLocation.Longitude, metadata.UserLocation.RadiusMile)

	// Convert to products.UserLocation
	return &products.UserLocation{
		Latitude:   metadata.UserLocation.Latitude,
		Longitude:  metadata.UserLocation.Longitude,
		RadiusMile: metadata.UserLocation.RadiusMile,
		Address:    metadata.UserLocation.Address,
	}
}
