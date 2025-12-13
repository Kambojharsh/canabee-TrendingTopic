package recommendations

import (
	"cannabee/pkg/db"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
)

type RecommendationService struct {
	db *sql.DB
}

func NewRecommendationService(database *sql.DB) *RecommendationService {
	return &RecommendationService{
		db: database,
	}
}

// SessionRecommendation represents a recommended session with similarity score
type SessionRecommendation struct {
	SessionID       string   `json:"session_id"`
	Summary         string   `json:"summary"`
	MatchingTags    []string `json:"matching_tags"`
	SimilarityScore int      `json:"similarity_score"`
}

// GetSimilarSessions finds sessions with similar tags and returns recommendations
func (rs *RecommendationService) GetSimilarSessions(ctx context.Context, sessionID string, limit int32) ([]SessionRecommendation, error) {
	queries := db.New(rs.db)

	// Get the session to find the user
	session, err := queries.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Get tags for the current user
	currentUserTags, err := queries.GetUserTags(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current user tags: %w", err)
	}

	if len(currentUserTags) == 0 {
		return []SessionRecommendation{}, nil
	}

	// Parse user tags from JSON
	var tagStrings []string
	if err := json.Unmarshal(currentUserTags, &tagStrings); err != nil {
		return nil, fmt.Errorf("failed to parse user tags: %w", err)
	}

	if len(tagStrings) == 0 {
		return []SessionRecommendation{}, nil
	}

	// For now, return empty recommendations as the similar sessions query
	// needs to be redesigned for user-level tags
	// TODO: Implement user-based similarity matching
	return []SessionRecommendation{}, nil
}

// GetSessionAnalytics provides analytics about a user's session patterns
func (rs *RecommendationService) GetSessionAnalytics(ctx context.Context, userID string) (*SessionAnalytics, error) {
	queries := db.New(rs.db)

	// Get user's sessions to count them
	sessions, err := queries.GetSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user sessions: %w", err)
	}

	// Get user's tags
	userTags, err := queries.GetUserTags(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tags: %w", err)
	}

	// Parse user tags from JSON
	var tagStrings []string
	if len(userTags) > 0 {
		if err := json.Unmarshal(userTags, &tagStrings); err != nil {
			log.Printf("Error parsing user tags: %v", err)
			tagStrings = []string{}
		}
	}

	// Create tag usage stats
	tagUsage := make([]TagUsage, len(tagStrings))
	for i, tag := range tagStrings {
		tagUsage[i] = TagUsage{
			Tag:   tag,
			Count: 1, // Each tag appears once per user in the new structure
		}
	}

	analytics := &SessionAnalytics{
		TotalSessions: len(sessions),
		PopularTags:   tagUsage,
	}

	return analytics, nil
}

// SessionAnalytics provides insights into user behavior
type SessionAnalytics struct {
	TotalSessions int        `json:"total_sessions"`
	PopularTags   []TagUsage `json:"popular_tags"`
}

// TagUsage represents how frequently a tag is used
type TagUsage struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}
