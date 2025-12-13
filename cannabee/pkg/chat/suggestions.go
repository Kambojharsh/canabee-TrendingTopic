package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

type SuggestionService struct {
	client *openai.Client
}

func NewSuggestionService(apiKey string) *SuggestionService {
	client := openai.NewClient(apiKey)
	return &SuggestionService{
		client: client,
	}
}

type Suggestion struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Message  string `json:"message"`
	Type     string `json:"type"`
	Priority int    `json:"priority"`
}

type SuggestionResponse struct {
	ShouldShowSuggestions bool         `json:"should_show_suggestions"`
	Suggestions           []Suggestion `json:"suggestions"`
	Reason                string       `json:"reason"`
}

// AnalyzeConversationForSuggestions analyzes the conversation to determine if suggestions should be shown
func (ss *SuggestionService) AnalyzeConversationForSuggestions(ctx context.Context, messages []ChatMessage) (*SuggestionResponse, error) {
	log.Printf("=== Analyzing Conversation for Suggestions ===")
	log.Printf("Input messages: %d", len(messages))

	// Get the last few messages for context
	lastUserMessage := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserMessage = messages[i].Content
			break
		}
	}
	log.Printf("Last user message: '%.100s...'", lastUserMessage)

	// Check if we should show suggestions based on conversation context
	shouldAnalyze := ss.shouldAnalyzeForSuggestions(ctx, messages)
	if !shouldAnalyze {
		log.Printf("Skipping suggestion analysis based on conversation context")
		return &SuggestionResponse{
			ShouldShowSuggestions: false,
			Suggestions:           []Suggestion{},
			Reason:                "Conversation context doesn't warrant suggestions",
		}, nil
	}

	// Create the analysis prompt
	systemPrompt := `You are an AI assistant that analyzes cannabis-related conversations to determine when to show contextual suggestions to users.

Your job is to detect when a user has confirmed or mentioned an issue/need and generate helpful suggestions that would appear near the chat input.

SHOW SUGGESTIONS when:
- User confirms they have a specific issue.
- User mentions a condition or need that cannabis might help with
- User is exploring cannabis for the first time for a specific purpose
- User has described their situation but hasn't asked for products yet (e.g. "I'm having trouble sleeping")

DO NOT show suggestions when:
- User is already asking for product recommendations
- User is in the middle of a product discussion
- User is asking general questions about cannabis
- User is just greeting or making casual conversation
- User is discussing non-cannabis topics

For each suggestion, provide:
- text: Short, actionable text (e.g., "Recommend me some products", "Tell me about dosing", "What should I avoid?")
- message: The actual message that will be sent when clicked
- type: "product_request", "information", or "help"
- priority: 1 (highest) to 5 (lowest)

Generate 3-5 relevant suggestions that would be most helpful based on the user's confirmed issue.

Examples:
- For sleep issues: "Recommend me some products", "Tell me about dosing for sleep", "What's the difference between indica and sativa?"
- For anxiety: "Show me calming products", "Tell me about CBD vs THC", "What should I start with as a beginner?"
- For pain: "Recommend pain relief products", "Tell me about topicals", "What's good for chronic pain?"

Respond in JSON format:
{
  "should_show_suggestions": boolean,
  "suggestions": [
    {
      "id": "unique_id",
      "text": "Short suggestion text",
      "message": "Full message to send when clicked",
      "type": "product_request" | "information" | "help",
      "priority": number
    }
  ],
  "reason": "Brief explanation of decision"
}`

	// Build conversation context
	contextMessages := []string{}
	messageCount := 0
	for i := len(messages) - 1; i >= 0 && messageCount < 6; i-- {
		msg := messages[i]
		if strings.TrimSpace(msg.Content) != "" {
			role := "User"
			if msg.Role == "assistant" {
				role = "AI"
			}
			contextMessages = append([]string{fmt.Sprintf("%s: %s", role, msg.Content)}, contextMessages...)
			messageCount++
		}
	}

	conversationContext := strings.Join(contextMessages, "\n")

	prompt := fmt.Sprintf(`You are analyzing a conversation to determine if contextual suggestions should be shown to help the user continue the conversation.

Conversation Context:
%s

SHOW suggestions when:
- User confirms they have a specific issue (sleep, anxiety, pain, focus, etc.)
- User mentions a condition or need that cannabis might help with
- User is exploring cannabis for the first time for a specific purpose
- User has described their situation but hasn't asked for products yet

DO NOT show suggestions when:
- User is already asking for product recommendations
- User is in the middle of a product discussion
- User is asking general questions about cannabis
- User is just greeting or making casual conversation
- User is discussing non-cannabis topics

For each suggestion, provide:
- text: Short, actionable text (e.g., "Recommend me some products", "Tell me about dosing", "What should I avoid?")
- message: The actual message that will be sent when clicked (FROM THE USER'S PERSPECTIVE - what the user would say)
- type: "product_request", "information", or "help"
- priority: 1 (highest) to 5 (lowest)

Generate 3-5 relevant suggestions that would be most helpful based on the user's confirmed issue.

Examples:
- For sleep issues: 
  * "Recommend me some products" -> "I'd like to see some cannabis products that might help with my sleep issues"
  * "Tell me about dosing for sleep" -> "What's the right dosing for sleep problems?"
  * "What's the difference between indica and sativa?" -> "Can you explain the difference between indica and sativa for sleep?"

- For anxiety: 
  * "Show me calming products" -> "I'd like to see some cannabis products that might help with anxiety"
  * "Tell me about CBD vs THC" -> "What's the difference between CBD and THC for anxiety?"
  * "What should I start with as a beginner?" -> "What would you recommend for someone new to cannabis for anxiety?"

- For pain: 
  * "Recommend pain relief products" -> "I'd like to see some cannabis products that might help with my pain"
  * "Tell me about topicals" -> "How do cannabis topicals work for pain relief?"
  * "What's good for chronic pain?" -> "What cannabis products work well for chronic pain?"

IMPORTANT: The "message" field should always be from the USER'S perspective - what they would say, not what the AI would say.

Respond in JSON format:
{
  "should_show_suggestions": boolean,
  "suggestions": [
    {
      "id": "unique_id",
      "text": "Short suggestion text",
      "message": "Full message to send when clicked (from user's perspective)",
      "type": "product_request" | "information" | "help",
      "priority": number
    }
  ],
  "reason": "Brief explanation of decision"
}`, conversationContext)

	log.Printf("Sending suggestion analysis request to OpenAI...")

	// Create the OpenAI request
	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   500,
	}

	// Debug log: Print the complete request being sent to OpenAI for suggestion analysis
	// verbose debug removed
	log.Printf("Model: %s", req.Model)
	log.Printf("Temperature: %.2f", req.Temperature)
	log.Printf("MaxTokens: %d", req.MaxTokens)
	log.Printf("Number of messages: %d", len(req.Messages))

	for i, msg := range req.Messages {
		log.Printf("Message %d - Role: %s", i+1, msg.Role)
		log.Printf("Message %d - Content: %s", i+1, msg.Content)
	}
	// verbose debug removed

	resp, err := ss.client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Printf("ERROR: OpenAI suggestion analysis failed: %v", err)
		return nil, fmt.Errorf("failed to analyze conversation for suggestions: %w", err)
	}

	if len(resp.Choices) == 0 {
		log.Printf("ERROR: No choices in OpenAI suggestion response")
		return nil, fmt.Errorf("no response from OpenAI suggestion analysis")
	}

	responseContent := resp.Choices[0].Message.Content
	log.Printf("OpenAI suggestion analysis response: %s", responseContent)

	// Parse the JSON response
	var suggestionResponse SuggestionResponse
	if err := json.Unmarshal([]byte(responseContent), &suggestionResponse); err != nil {
		log.Printf("ERROR: Failed to parse OpenAI suggestion response: %v", err)
		return nil, fmt.Errorf("failed to parse suggestion response: %w", err)
	}

	// Generate unique IDs for suggestions if not provided
	for i := range suggestionResponse.Suggestions {
		if suggestionResponse.Suggestions[i].ID == "" {
			suggestionResponse.Suggestions[i].ID = fmt.Sprintf("suggestion_%d", i+1)
		}
	}

	log.Printf("=== Suggestion Analysis Complete ===")
	log.Printf("Should show suggestions: %t", suggestionResponse.ShouldShowSuggestions)
	log.Printf("Number of suggestions: %d", len(suggestionResponse.Suggestions))
	log.Printf("Reason: %s", suggestionResponse.Reason)

	return &suggestionResponse, nil
}

// shouldAnalyzeForSuggestions determines if we should analyze the conversation for suggestions using OpenAI
func (ss *SuggestionService) shouldAnalyzeForSuggestions(ctx context.Context, messages []ChatMessage) bool {
	log.Printf("=== Determining if should analyze for suggestions using OpenAI ===")

	// Get the last few messages for context
	contextMessages := []string{}
	messageCount := 0
	for i := len(messages) - 1; i >= 0 && messageCount < 6; i-- {
		msg := messages[i]
		if strings.TrimSpace(msg.Content) != "" {
			role := "User"
			if msg.Role == "assistant" {
				role = "AI"
			}
			contextMessages = append([]string{fmt.Sprintf("%s: %s", role, msg.Content)}, contextMessages...)
			messageCount++
		}
	}

	conversationContext := strings.Join(contextMessages, "\n")

	systemPrompt := `You are an AI assistant that determines whether a conversation context warrants showing contextual suggestions to help the user continue the conversation.

Your job is to analyze the conversation and determine if the user would benefit from seeing suggestion pills that help them continue the conversation.

ANALYZE FOR SUGGESTIONS when:
- User has mentioned or confirmed they have a specific issue or need (sleep, anxiety, pain, focus, etc.)
- User is exploring cannabis for the first time for a specific purpose
- User has described their situation but hasn't asked for products yet
- User seems to need guidance on what to ask next
- User has expressed a condition or need that cannabis might help with

DO NOT analyze for suggestions when:
- User is already asking for product recommendations
- User is in the middle of a detailed product discussion
- User is asking general questions about cannabis
- User is just greeting or making casual conversation
- User is discussing non-cannabis topics
- The conversation is very early stage with no clear context

Respond with only "YES" or "NO" followed by a brief reason.`

	prompt := fmt.Sprintf(`Based on this conversation context, should we show contextual suggestions to help the user continue the conversation?

Conversation Context:
%s

Respond with only "YES" or "NO" followed by a brief reason.`, conversationContext)

	log.Printf("Sending shouldAnalyze request to OpenAI...")

	// Create the OpenAI request
	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		Temperature: 0.1,
		MaxTokens:   50,
	}

	// Debug log: Print the complete request being sent to OpenAI for shouldAnalyze
	// verbose debug removed
	log.Printf("Model: %s", req.Model)
	log.Printf("Temperature: %.2f", req.Temperature)
	log.Printf("MaxTokens: %d", req.MaxTokens)
	log.Printf("Number of messages: %d", len(req.Messages))

	for i, msg := range req.Messages {
		log.Printf("Message %d - Role: %s", i+1, msg.Role)
		log.Printf("Message %d - Content: %s", i+1, msg.Content)
	}
	// verbose debug removed

	resp, err := ss.client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Printf("ERROR: OpenAI shouldAnalyze failed: %v", err)
		// Fallback to true to be safe
		return true
	}

	if len(resp.Choices) == 0 {
		log.Printf("ERROR: No choices in OpenAI shouldAnalyze response")
		// Fallback to true to be safe
		return true
	}

	responseContent := strings.TrimSpace(resp.Choices[0].Message.Content)
	log.Printf("OpenAI shouldAnalyze response: %s", responseContent)

	// Parse the response - expecting "YES" or "NO" at the beginning
	shouldAnalyze := strings.HasPrefix(strings.ToUpper(responseContent), "YES")

	log.Printf("=== Should analyze for suggestions: %t ===", shouldAnalyze)
	return shouldAnalyze
}
