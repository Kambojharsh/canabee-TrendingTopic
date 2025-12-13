package chat

import (
	"cannabee/pkg/db"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type ContextService struct {
	db           *sql.DB
	openaiClient *OpenAIClient
}

func NewContextService(database *sql.DB, openaiClient *OpenAIClient) *ContextService {
	return &ContextService{
		db:           database,
		openaiClient: openaiClient,
	}
}

// UserSessionContext holds conversation history context from previous sessions
// This context is used throughout the entire session to enhance all AI responses
type UserSessionContext struct {
	RecentSummaries []string `json:"recent_summaries"` // Summaries from last 3 sessions for conversation continuity
	RecentTags      []string `json:"recent_tags"`      // Topics from last 3 sessions for personalized responses
	SessionCount    int      `json:"session_count"`    // Total number of previous sessions
}

// GetUserSessionContext retrieves conversation history context from the last 3 sessions
// This context is used throughout the entire session to enhance all AI responses with continuity
func (cs *ContextService) GetUserSessionContext(ctx context.Context, userID string) (*UserSessionContext, error) {
	queries := db.New(cs.db)

	// Get last 3 completed sessions for the user
	recentSessions, err := queries.GetLastNSessionsForUser(ctx, db.GetLastNSessionsForUserParams{
		UserID: userID,
		Limit:  3,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get recent sessions: %w", err)
	}

	// Build conversation history context for this user
	sessionContext := &UserSessionContext{
		RecentSummaries: make([]string, 0, len(recentSessions)),
		RecentTags:      []string{},
		SessionCount:    len(recentSessions),
	}

	if len(recentSessions) == 0 {
		return sessionContext, nil
	}

	// Get summaries and tags from recent sessions
	sessionIDs := make([]string, len(recentSessions))
	for i, session := range recentSessions {
		sessionIDs[i] = session.SessionID

		// Get summary from session (now stored directly in sessions table)
		if session.Summary.Valid && session.Summary.String != "" {
			sessionContext.RecentSummaries = append(sessionContext.RecentSummaries, session.Summary.String)
		}
	}

	// Get user's tags (since tags are now stored at user level)
	userTags, err := queries.GetUserTags(ctx, userID)
	if err == nil && len(userTags) > 0 {
		var tagStrings []string
		if err := json.Unmarshal(userTags, &tagStrings); err == nil {
			sessionContext.RecentTags = tagStrings
		}
	}

	return sessionContext, nil
}

// GeneratePersonalizedGreeting creates a personalized greeting based on user's session history and session type
func (cs *ContextService) GeneratePersonalizedGreeting(ctx context.Context, userContext *UserSessionContext, sessionType string, userConfig map[string]interface{}) (string, error) {
	// Create context for AI to generate personalized greeting
	var sessionTypeContext string
	switch sessionType {
	case "guide_me":
		sessionTypeContext = "This is a 'Guide Me' session where the user wants personalized product recommendations and guidance."
	case "my_journey":
		sessionTypeContext = "This is a 'My Journey' session where the user wants to explore and learn about cannabis on their own terms."
	case "ask_bee":
		sessionTypeContext = "This is a 'Ask Bee' session where the user wants to ask questions about cannabis."
	default:
		sessionTypeContext = "This is a general cannabis consultation session."
	}

	// Determine if this is a new user or returning user
	var userContextInfo string
	if userContext.SessionCount == 0 {
		userContextInfo = "This is a new user with no previous sessions."
	} else if len(userContext.RecentSummaries) == 0 {
		userContextInfo = "This is a returning user but no recent session summaries are available."
	} else {
		userContextInfo = fmt.Sprintf("This is a returning user with %d previous sessions.", userContext.SessionCount)
	}

	// Build user preferences section if config is provided
	var userPreferencesSection string
	if userConfig != nil && len(userConfig) > 0 {
		var preferencesList []string
		for key, value := range userConfig {
			preferencesList = append(preferencesList, fmt.Sprintf("- %s: %v", key, value))
		}
		userPreferencesSection = fmt.Sprintf("\n\nUser Preferences:\n%s", strings.Join(preferencesList, "\n"))
	}

	// Special handling for my_journey sessions with user config - generate comprehensive personalized summary
	if sessionType == "my_journey" && userConfig != nil && len(userConfig) > 0 {
		// Build a comprehensive, personalized journey summary prompt
		contextPrompt := fmt.Sprintf(`Generate a comprehensive, personalized welcome message for a cannabis user based on their profile. This should be a detailed, warm summary that acknowledges their preferences and sets the stage for exploration.

Start with: "Hey there 👋 — based on your profile, I've got a good sense of what kind of cannabis experience fits you best."

Then create a detailed paragraph that naturally weaves together:
- Their experience level (beginner/intermediate/experienced)
- Their main interests/goals (pain relief, sleep, anxiety, social fun, creativity, etc.)
- Their preferred effects (energizing, uplifting, relaxing, balanced, etc.)
- Their consumption timing (daytime, evenings, occasionally, daily, etc.)
- Their preferred consumption methods (vaping, edibles, drinks, topicals, flower, tinctures, oils, etc.)
- Their potency preferences (low, medium, high)
- Their THC dose preferences (low, normal, high)
- Their effect duration preferences
- Their flavor preferences (if mentioned)
- Their discretion preferences (social, solo, discreet, etc.)
- Their price considerations (if mentioned)
- Their interest in cannabis education

End with: "Ready to start exploring some options that fit your style? I can show you products that match your profile or help you learn more about dosage, methods, or effects — what would you like to do first?"

User Preferences:
%s

Recent session summaries (for context):
%s

Recent topics/tags: %s

Guidelines:
- Write in a warm, conversational, friendly tone
- Use natural language that flows smoothly
- Make it feel personal and acknowledge their unique profile
- Include emojis naturally (👋)
- Be comprehensive but not overwhelming
- Reference their preferences naturally without sounding robotic
- End with an engaging question that invites them to explore

Generate the comprehensive personalized welcome message now:`,
			userPreferencesSection,
			strings.Join(userContext.RecentSummaries, "\n"),
			strings.Join(userContext.RecentTags, ", "))

		greeting, err := cs.openaiClient.GetResponse(ctx, []ChatMessage{
			{
				Role:    "user",
				Content: contextPrompt,
			},
		})
		if err != nil {
			log.Printf("Error generating personalized journey greeting: %v", err)
			// Fallback to simple greeting
			return "Hey! I'm **Cannabee**, your friendly cannabis guide! What can I help you with today?", nil
		}

		return strings.TrimSpace(greeting), nil
	}

	// Build guidelines for other session types
	guidelines := `- Sound like a friendly person, not formal assistant
- Reference previous topics naturally if relevant (only if there are recent summaries/tags)
- Ask a simple, caring follow-up question
- Keep it short and conversational
- Don't repeat their words back to them
- Use "Welcome back!" or "Hey again!" for returning users, "Hey!" for new users
- Tailor the greeting to the session type (Guide Me = recommendations focus, My Journey = exploration focus)`

	contextPrompt := fmt.Sprintf(`Generate a warm, natural greeting for a cannabis consultation user. Keep it conversational and brief (2-3 sentences max). Use bee persona with 🐝 emoji.

Session Type: %s
User Context: %s

Recent session summaries:
%s

Recent topics/tags: %s%s

Guidelines:
%s

Generate the greeting now:`,
		sessionTypeContext,
		userContextInfo,
		strings.Join(userContext.RecentSummaries, "\n"),
		strings.Join(userContext.RecentTags, ", "),
		userPreferencesSection,
		guidelines)

	greeting, err := cs.openaiClient.GetResponse(ctx, []ChatMessage{
		{
			Role:    "user",
			Content: contextPrompt,
		},
	})
	if err != nil {
		log.Printf("Error generating personalized greeting: %v", err)
		// Fallback to simple greeting
		return "Hey! I'm **Cannabee**, your friendly cannabis guide! What can I help you with today?", nil
	}

	return strings.TrimSpace(greeting), nil
}

// GenerateSessionTypeGreeting creates a greeting based on session type without using journey history
func (cs *ContextService) GenerateSessionTypeGreeting(ctx context.Context, sessionType string) (string, error) {
	// Create context for AI to generate session-type-specific greeting without history
	var sessionTypeContext string
	switch sessionType {
	case "guide_me":
		sessionTypeContext = "This is a 'Guide Me' session where the user wants personalized product recommendations and guidance. Generate a warm, welcoming greeting that focuses on helping them find the perfect products for their needs."
	case "my_journey":
		sessionTypeContext = "This is a 'My Journey' session where the user wants to explore and learn about cannabis on their own terms. Generate a warm, welcoming greeting that focuses on supporting their personal exploration and learning."
	case "ask_bee":
		sessionTypeContext = "This is a 'Ask Bee' session where the user wants to ask questions about cannabis. Generate a warm, welcoming greeting that focuses on helping them with their questions."
	default:
		sessionTypeContext = "This is a general cannabis consultation session. Generate a warm, welcoming greeting that offers to help with their cannabis needs."
	}

	contextPrompt := fmt.Sprintf(`Generate a warm, natural greeting for a cannabis consultation user. Keep it conversational and brief (2-3 sentences max). Use bee persona with 🐝 emoji.

Session Type: %s

Guidelines:
- Sound like a friendly person, not formal assistant
- Ask a simple, caring follow-up question
- Keep it short and conversational
- Use "Hey!" or "Welcome!" 
- Tailor the greeting to the session type (Guide Me = recommendations focus, My Journey = exploration focus)

Generate the greeting now:`,
		sessionTypeContext)

	greeting, err := cs.openaiClient.GetResponse(ctx, []ChatMessage{
		{
			Role:    "user",
			Content: contextPrompt,
		},
	})
	if err != nil {
		log.Printf("Error generating session-type greeting: %v", err)
		// Fallback to simple greeting
		return "Hey! I'm **Cannabee**, your friendly cannabis guide! What can I help you with today?", nil
	}

	return strings.TrimSpace(greeting), nil
}

// CleanupOldUserTags removes tags from sessions older than the last 3 (lazy cleanup)
func (cs *ContextService) CleanupOldUserTags(ctx context.Context, userID string) error {
	// Note: Tags are now stored at user level, so no session-level tag cleanup is needed
	// Tags persist across all sessions for a user, which is the intended behavior
	log.Printf("Tag cleanup not needed - tags are now stored at user level for user %s", userID)
	return nil
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
