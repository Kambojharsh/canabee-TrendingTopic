package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client *openai.Client
}

func NewOpenAIClient(apiKey string) *OpenAIClient {
	client := openai.NewClient(apiKey)
	return &OpenAIClient{
		client: client,
	}
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GetMainConversationalSystemPrompt returns the main system prompt for Cannabee conversations
func GetMainConversationalSystemPrompt() string {
	return `🐝 You are **Cannabee**, a friendly and knowledgeable cannabis guide with a warm bee persona. Your goal is to help users find the perfect cannabis products through natural, comfortable conversation.

## 🌼 Core Personality & Communication Style:
• **Conversational & Natural**: Talk like a knowledgeable friend, not a formal assistant
• **Concise & Focused**: Keep responses SHORT and to the point - NO long paragraphs
• **Bee-themed but Subtle**: Use light bee references (🐝, 🍯, hive, buzz) but don't overdo it
• **CRITICAL: ONE Question at a Time**: NEVER EVER ask multiple questions in one response - this is strictly forbidden
• **NO Repetitive Acknowledgements**: Don't repeat back what the user just said - move forward naturally
• **Thoughtful Discovery**: Take time to understand user needs before recommending products

## 🚫 STRICT ONE-QUESTION RULE:
• NEVER ask more than ONE question per response
• Instead of "What effects are you looking for? And do you prefer edibles or flower?" 
• Say: "What effects are you looking for?" (Wait for answer, then ask about format)
• If you need multiple pieces of info, ask them one at a time across multiple messages
• This rule is NON-NEGOTIABLE - violating it makes conversations overwhelming

## 🎯 Conversation Flow:
1. **Listen & Learn**: Understand their needs WITHOUT repeating their words
2. **Ask ONE Smart Question**: One thoughtful question that helps narrow preferences
3. **Gather Context**: Learn about experience level, preferences, and specific needs
4. **Show Products Thoughtfully**: Only after understanding their situation well
5. **Be Helpful**: Provide useful information based on your knowledge base

## 🎯 Discovery Approach & Goal:
**Goal**: Help users feel informed and confident about their purchase decision

**Discovery Approach**: When users don't know what they want, explore their situation before suggesting options:
- Ask about their specific situation and what they're hoping cannabis will help with
- When they say "I don't know," ask about context - but if they mention specific needs, you can suggest relevant formats
- Help them articulate what matters to them

**Need Specificity**: Break down broad categories:
- Sleep → falling asleep vs staying asleep? What's causing it?
- Anxiety → social, general worry, panic, racing thoughts?
- Pain → chronic/acute? What type?
- Collect these specifics even if products don't have perfect matches - this data helps us improve

**Natural Conversation**:
- Acknowledge without always summarizing back
- Advance conversation (not just echo + next question)
- Adapt style to what user needs (detail vs brevity)
- If user mentions specific needs (focus, energy, daytime use), suggest relevant formats like vape pens early
- For vague requests, gather more context first

## 🌿 CANNABIS ASSUMPTION & CONTEXT PRIORITY:
• **ALWAYS ASSUME**: Users are in the app specifically for cannabis solutions - NEVER ask "would you consider trying cannabis products"
• **Cannabis First**: When discussing products or effects, ALWAYS explain cannabis-related benefits and effects FIRST
• **Secondary Ingredients**: Additional ingredients (ginger, peppermint, etc.) should be mentioned as secondary context only
• **Clear Separation**: Maintain clear distinction between cannabis effects vs. supplementary ingredients
• **No Cannabis Questioning**: Don't question their interest in cannabis - focus on finding the right cannabis solution for them

## 🔍 Discovery Questions to Ask First:
When users mention needs/effects, ask qualifying questions like:
• "Are you new to cannabis or have you used it before?"
• "What consumption method do you usually prefer?" (if experienced)
• "How important is avoiding THC vs having some THC?"
• "Are you looking for something for daytime or nighttime use?"
• "What's your main goal - medical relief or recreational enjoyment?"
• "Have you tried anything similar before? How did it work?"

## 🚀 Quick Format Suggestions:
For specific use cases, suggest relevant formats early:
• Focus/Energy/Work → "Vape pens might be perfect for that - quick onset, precise dosing"
• Sleep → "Edibles or tinctures work great for sleep - longer lasting effects"
• Pain → "Topicals for localized relief, or edibles for body-wide effects"
• Social/Discrete → "Vape pens are discreet and fast-acting"

## 🚀 When to Show Products:
Only offer products after gathering context about:
• Experience level (beginner vs experienced)
• Consumption preferences (edibles, flower, vapes, etc.)
• THC tolerance/preferences
• Timing of use (day/night)
• Specific goals beyond general effects

**Thoughtful responses:**
• "I'd love to help you find something perfect! Are you new to cannabis?" 🐝
• "Let me understand your preferences better first - what's your experience level?" 🍯
• "Before I show you options, do you prefer edibles or other methods?"
• "I want to find the right fit for you - is this for daytime or evening use?"

## 🍯 Returning User Experience:
For returning users, acknowledge them warmly:
• "Welcome back! Still looking for [last topic]?"
• "Good to see you again! How did [last recommendation] work?"
• "Back for more? Want something similar or different this time?"

## 🌿 CANNABIS CONSUMPTION METHODS & PRODUCT TYPES:
When users mention specific consumption methods, ALWAYS preserve them in your responses and product recommendations:

**Inhalation Methods:**
- Smoking: Joints/pre-rolls, pipes, bongs, blunts
- Vaporizing: Dry herb vaporizers, concentrate vaporizers, cartridges

**Oral Consumption:**
- Edibles: Gummies, chocolates, baked goods, beverages
- Tinctures/Oils: Sublingual (under tongue) or added to food/drinks
- Capsules: Precise dosing

**Topical Application:**
- Creams, balms, lotions, patches

**Other Methods:**
- Dabbing: High-potency concentrates
- Suppositories: Medical use for specific conditions

## 🎯 PRODUCT TYPE ACCURACY:
• If user asks for "tinctures for sleep" → recommend tinctures specifically
• If user asks for "oils for pain" → recommend oils specifically  
• If user asks for "edibles for anxiety" → recommend edibles specifically
• NEVER suggest tinctures when user asks for oils, or vice versa
• Preserve the exact consumption method the user mentioned

## 📚 Knowledge & Safety:
• Ground responses in knowledge base information
• If no specific info available: "I don't have details on that, but..."
• Brief disclaimers: "For educational purposes only. [T&C Link]"
• NO lengthy medical justifications
• Encourage healthcare consultation when appropriate

## 🚫 Strict Rules - NEVER:
• Ask multiple questions in one response (MOST IMPORTANT RULE)
• Ask "would you consider trying cannabis products" or question their cannabis interest
• Repeat user's words back to them
• Write long, overwhelming responses
• Use lengthy medical disclaimers
• Sound formal or robotic
• Make medical claims without knowledge base support
• Discuss non-cannabis ingredients before cannabis effects

## 📋 Response Format:
• Start with engagement (no acknowledgement)
• Provide helpful info (brief)
• End with EXACTLY ONE question OR product offer
• Keep total response under 3 sentences when possible
• If you need multiple pieces of info, ask them across separate messages

Remember: You're a helpful bee guide 🐝 having a natural conversation to help users find their perfect cannabis match quickly! ONE QUESTION AT A TIME ALWAYS! Assume they want cannabis solutions and prioritize cannabis effects first!`
}

func (oc *OpenAIClient) GetStreamResponse(ctx context.Context, messages []ChatMessage) (*openai.ChatCompletionStream, error) {
	return oc.GetStreamResponseWithSystemPrompt(ctx, messages, GetMainConversationalSystemPrompt())
}

func (oc *OpenAIClient) GetStreamResponseWithSystemPrompt(ctx context.Context, messages []ChatMessage, systemPrompt string) (*openai.ChatCompletionStream, error) {
	log.Printf("Creating AI stream response")

	// Add main system prompt if not already present
	if len(messages) == 0 || messages[0].Role != "system" {
		systemMessage := ChatMessage{
			Role:    "system",
			Content: systemPrompt,
		}
		messages = append([]ChatMessage{systemMessage}, messages...)
		log.Printf("Added system message to beginning of conversation")
	} else {
		log.Printf("System message already present, replacing with custom prompt")
		messages[0].Content = systemPrompt
	}

	// Convert our messages to OpenAI format and validate content
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		// Validate and clean the message content
		content := msg.Content
		if content == "" {
			log.Printf("Warning: Empty content in message %d (role: %s), replacing with default", i, msg.Role)
			if msg.Role == "user" {
				content = "Hello"
			} else if msg.Role == "system" {
				content = "You are a helpful AI assistant."
			} else {
				content = "I'm here to help."
			}
		}

		// Additional validation to ensure content is not null or problematic
		if len(strings.TrimSpace(content)) == 0 {
			log.Printf("Warning: Whitespace-only content in message %d (role: %s), replacing with default", i, msg.Role)
			content = "I'm here to help."
		}

		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: content,
		}

		// Log the final message for debugging
		log.Printf("Message %d: role=%s, content_length=%d, content_preview=%s",
			i, msg.Role, len(content), content[:min(50, len(content))])
	}

	req := openai.ChatCompletionRequest{
		Model:       openai.GPT3Dot5Turbo,
		Messages:    openaiMessages,
		Stream:      true,
		Temperature: 0.2,
	}

	// Debug log: Print the complete request being sent to OpenAI
	// verbose debug removed
	log.Printf("Model: %s", req.Model)
	log.Printf("Temperature: %.2f", req.Temperature)
	log.Printf("Stream: %t", req.Stream)
	log.Printf("Number of messages: %d", len(req.Messages))

	for i, msg := range req.Messages {
		log.Printf("Message %d - Role: %s", i+1, msg.Role)
		log.Printf("Message %d - Content: %s", i+1, msg.Content)
	}
	// verbose debug removed

	stream, err := oc.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion stream: %w", err)
	}

	return stream, nil
}

func (oc *OpenAIClient) GetResponse(ctx context.Context, messages []ChatMessage) (string, error) {
	// Add main system prompt if not already present
	if len(messages) == 0 || messages[0].Role != "system" {
		systemMessage := ChatMessage{
			Role:    "system",
			Content: GetMainConversationalSystemPrompt(),
		}
		messages = append([]ChatMessage{systemMessage}, messages...)
	}

	// Convert our messages to OpenAI format and validate content
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		// Validate and clean the message content
		content := msg.Content
		if content == "" {
			log.Printf("Warning: Empty content in message %d (role: %s), replacing with default", i, msg.Role)
			if msg.Role == "user" {
				content = "Hello"
			} else if msg.Role == "system" {
				content = "You are a helpful AI assistant."
			} else {
				content = "I'm here to help."
			}
		}

		// Additional validation to ensure content is not null or problematic
		if len(strings.TrimSpace(content)) == 0 {
			log.Printf("Warning: Whitespace-only content in message %d (role: %s), replacing with default", i, msg.Role)
			content = "I'm here to help."
		}

		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: content,
		}

		// Log the final message for debugging
		log.Printf("Message %d: role=%s, content_length=%d, content_preview=%s",
			i, msg.Role, len(content), content[:min(50, len(content))])
	}

	req := openai.ChatCompletionRequest{
		Model:    openai.GPT3Dot5Turbo,
		Messages: openaiMessages,
	}

	// Debug log: Print the complete request being sent to OpenAI
	// verbose debug removed
	log.Printf("Model: %s", req.Model)
	log.Printf("Number of messages: %d", len(req.Messages))

	for i, msg := range req.Messages {
		log.Printf("Message %d - Role: %s", i+1, msg.Role)
		log.Printf("Message %d - Content: %s", i+1, msg.Content)
	}
	// verbose debug removed

	resp, err := oc.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	return resp.Choices[0].Message.Content, nil
}

func GetInitialGreeting() string {
	return "Hey! I'm **Cannabee** 🐝, your friendly cannabis guide! I'm here to help you navigate the world of cannabis and find what works best for you.\n\nWhat brings you here today? Whether you're curious about cannabis, looking for something specific, or just want to learn more - I'm here to help! 🍯"
}

// TagsResponse represents the JSON response format for tags
type TagsResponse struct {
	Tags []string `json:"tags"`
}

// GenerateSessionTags analyzes the chat history and generates relevant tags
func (oc *OpenAIClient) GenerateSessionTags(ctx context.Context, messages []ChatMessage, tagsCount int) ([]string, error) {
	// Default to 5 tags if not specified
	if tagsCount <= 0 {
		tagsCount = 5
	}

	// Create a conversation summary for tag generation
	var conversation strings.Builder
	for _, msg := range messages {
		// Skip empty messages
		if strings.TrimSpace(msg.Content) == "" {
			log.Printf("Warning: Skipping empty message in tag generation (role: %s)", msg.Role)
			continue
		}

		if msg.Role == "user" {
			conversation.WriteString("User: " + msg.Content + "\n")
		} else if msg.Role == "assistant" {
			conversation.WriteString("Assistant: " + msg.Content + "\n")
		}
	}

	// If no meaningful conversation content, return empty tags
	if conversation.Len() == 0 {
		log.Printf("Warning: No meaningful conversation content for tag generation")
		return []string{}, nil
	}

	var tagCountText string
	if tagsCount == 1 {
		tagCountText = "1 relevant tag/keyword"
	} else {
		tagCountText = fmt.Sprintf("%d relevant tags/keywords", tagsCount)
	}

	prompt := fmt.Sprintf(`Analyze the following conversation and generate %s that describe the main topics, themes, or subjects discussed.

Return the response as a JSON object with a "tags" array containing only the tag strings (no explanations).

Example format:
{"tags": ["cannabis", "medical", "anxiety", "dosage"]}

Conversation:
%s

Focus on:
- Main topics discussed
- Cannabis-related terms (strains, products, effects)
- Medical conditions or symptoms
- Usage methods or consumption
- User concerns or interests

Normalize tags to lowercase and use common terminology.`, tagCountText, conversation.String())

	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.3, // Lower temperature for more consistent tag generation
	}

	// Debug log: Print the complete request being sent to OpenAI for tag generation
	// verbose debug removed
	log.Printf("Model: %s", req.Model)
	log.Printf("Temperature: %.2f", req.Temperature)
	log.Printf("Number of messages: %d", len(req.Messages))

	for i, msg := range req.Messages {
		log.Printf("Message %d - Role: %s", i+1, msg.Role)
		log.Printf("Message %d - Content: %s", i+1, msg.Content)
	}
	// verbose debug removed

	resp, err := oc.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tags: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response choices returned for tag generation")
	}

	// Parse the JSON response
	var tagsResp TagsResponse
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(content), &tagsResp); err != nil {
		// Fallback: try to extract tags from a non-JSON response
		return oc.extractTagsFromText(content), nil
	}

	// Normalize tags and limit to requested count
	normalizedTags := make([]string, 0, len(tagsResp.Tags))
	for _, tag := range tagsResp.Tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if normalized != "" && len(normalized) <= 100 { // Ensure it fits our DB constraint
			normalizedTags = append(normalizedTags, normalized)
			// Limit to requested count
			if len(normalizedTags) >= tagsCount {
				break
			}
		}
	}

	return normalizedTags, nil
}

// GenerateSessionTitle creates a concise, meaningful title for the chat session
func (oc *OpenAIClient) GenerateSessionTitle(ctx context.Context, messages []ChatMessage) (string, error) {
	// Create a conversation summary for title generation
	var conversation strings.Builder
	for _, msg := range messages {
		// Skip empty messages
		if strings.TrimSpace(msg.Content) == "" {
			log.Printf("Warning: Skipping empty message in title generation (role: %s)", msg.Role)
			continue
		}

		if msg.Role == "user" {
			conversation.WriteString("User: " + msg.Content + "\n")
		} else if msg.Role == "assistant" {
			conversation.WriteString("Assistant: " + msg.Content + "\n")
		}
	}

	// If no meaningful conversation content, return generic title
	if conversation.Len() == 0 {
		log.Printf("Warning: No meaningful conversation content for title generation")
		return "Cannabis Chat Session", nil
	}

	prompt := fmt.Sprintf(`Generate a concise, descriptive title for this cannabis consultation conversation. The title should be:
- Maximum 6 words
- Descriptive of the main topic or need
- Professional but friendly
- No quotes or special characters
- Focused on the user's primary interest or question

Examples:
- "Sleep Support Cannabis Recommendations"
- "Anxiety Relief Product Guidance"
- "Beginner Cannabis Product Advice"
- "Pain Management Cannabis Options"
- "Focus Enhancement Product Search"

Conversation:
%s

Generate only the title (no explanation):`, conversation.String())

	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.3, // Lower temperature for more consistent title generation
	}

	// Debug log: Print the complete request being sent to OpenAI for title generation
	// verbose debug removed
	log.Printf("Model: %s", req.Model)
	log.Printf("Temperature: %.2f", req.Temperature)
	log.Printf("Number of messages: %d", len(req.Messages))

	for i, msg := range req.Messages {
		log.Printf("Message %d - Role: %s", i+1, msg.Role)
		log.Printf("Message %d - Content: %s", i+1, msg.Content)
	}
	// verbose debug removed

	resp, err := oc.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate title: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned for title generation")
	}

	title := strings.TrimSpace(resp.Choices[0].Message.Content)

	// Clean up the title - remove quotes if present
	title = strings.Trim(title, "\"'")

	return title, nil
}

// GenerateSessionSummary creates a concise summary of the entire chat session
func (oc *OpenAIClient) GenerateSessionSummary(ctx context.Context, messages []ChatMessage) (string, error) {
	// Create a conversation summary for summary generation
	var conversation strings.Builder
	for _, msg := range messages {
		// Skip empty messages
		if strings.TrimSpace(msg.Content) == "" {
			log.Printf("Warning: Skipping empty message in summary generation (role: %s)", msg.Role)
			continue
		}

		if msg.Role == "user" {
			conversation.WriteString("User: " + msg.Content + "\n")
		} else if msg.Role == "assistant" {
			conversation.WriteString("Assistant: " + msg.Content + "\n")
		}
	}

	// If no meaningful conversation content, return empty summary
	if conversation.Len() == 0 {
		log.Printf("Warning: No meaningful conversation content for summary generation")
		return "", nil
	}

	prompt := fmt.Sprintf(`Summarize the following conversation in 2-3 concise sentences. Focus on:
- The main topics or questions the user had
- Key information or recommendations provided
- Overall context of the conversation

Conversation:
%s

Provide a clear, helpful summary that captures the essence of this chat session:`, conversation.String())

	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.4, // Balanced temperature for good summarization
	}

	// Debug log: Print the complete request being sent to OpenAI for summary generation
	// verbose debug removed
	log.Printf("Model: %s", req.Model)
	log.Printf("Temperature: %.2f", req.Temperature)
	log.Printf("Number of messages: %d", len(req.Messages))

	for i, msg := range req.Messages {
		log.Printf("Message %d - Role: %s", i+1, msg.Role)
		log.Printf("Message %d - Content: %s", i+1, msg.Content)
	}
	// verbose debug removed

	resp, err := oc.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned for summary generation")
	}

	summary := strings.TrimSpace(resp.Choices[0].Message.Content)
	return summary, nil
}

// extractTagsFromText is a fallback method to extract tags from non-JSON responses
func (oc *OpenAIClient) extractTagsFromText(text string) []string {
	var tags []string
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for bullet points or comma-separated values
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") {
			tag := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "-"), "*"))
			if tag != "" {
				tags = append(tags, strings.ToLower(tag))
			}
		} else if strings.Contains(line, ",") {
			// Try to split by comma
			parts := strings.Split(line, ",")
			for _, part := range parts {
				tag := strings.TrimSpace(part)
				if tag != "" && !strings.Contains(tag, " ") { // Simple heuristic for single words
					tags = append(tags, strings.ToLower(tag))
				}
			}
		}
	}

	return tags
}

// ProductRecommendationDecision represents the decision of whether to show product recommendations
type ProductRecommendationDecision struct {
	ShouldShowRecommendations bool   `json:"should_show_recommendations"`
	RecommendationQuery       string `json:"recommendation_query"`
	Reason                    string `json:"reason"`
}

// ShouldShowProductRecommendations analyzes conversation and decides if product recommendations are needed
func (oc *OpenAIClient) ShouldShowProductRecommendations(ctx context.Context, messages []ChatMessage) (*ProductRecommendationDecision, error) {
	log.Printf("=== Analyzing Conversation for Product Recommendations ===")
	log.Printf("Input messages: %d", len(messages))

	// Log conversation context for debugging
	if len(messages) > 0 {
		lastUserMsg := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				lastUserMsg = messages[i].Content
				break
			}
		}
		log.Printf("Last user message: '%.100s...'", lastUserMsg)

		if len(messages) > 0 && messages[len(messages)-1].Role != "user" {
			log.Printf("Last message is not from user (role: %s), skipping recommendations", messages[len(messages)-1].Role)
			return &ProductRecommendationDecision{
				ShouldShowRecommendations: false,
				RecommendationQuery:       "",
				Reason:                    "Last message is not from user",
			}, nil
		}
	}

	// Create a prompt to analyze the conversation and decide if product recommendations are needed
	systemPrompt := `You are an AI assistant that determines when to show cannabis product recommendations. You should be thoughtful and gather sufficient context before showing products to provide the best user experience.

Analyze the conversation and determine if the user would benefit from seeing product recommendations.

SHOW RECOMMENDATIONS when:
- User explicitly asks for product recommendations (uses words like "recommend", "suggest", "what should I buy", "what products")
- User gives positive responses to AI asking about product recommendations ("yes", "sure", "yep", "okay", "that would be great", etc.)
- User asks to see products from the catalogue/catalog
- User mentions wanting to buy, try, or shop for something
- AI has already gathered context about user's experience level, preferences, and needs
- User has provided specific details about their consumption preferences (edibles, flower, vapes, etc.)
- User has indicated their experience level and the conversation has progressed beyond initial discovery

DO NOT show recommendations when:
- User just mentions effects or conditions without context (sleep, pain, anxiety, focus, etc.) - these need qualifying questions first
- User asks "what's good for..." or "what should I use for..." - these need discovery questions about experience, preferences, timing
- User asks about specific types of products generally - need to understand their experience level first
- User asks about dosage/consumption methods without context about their experience
- User describes symptoms or conditions - need to understand their cannabis experience first
- User asks about effects or benefits of cannabis generally - educational response needed, not products
- User shows interest in exploring cannabis options - need discovery questions first
- User is asking about laws, regulations, or legal matters
- User is asking about growing or cultivation
- Conversation is just initial greeting (first message)
- User is asking about non-cannabis topics
- User is clearly just chatting without cannabis interest
- AI hasn't gathered enough context about user's experience level, preferences, and specific needs

IMPORTANT: If you decide to show recommendations, you MUST provide a meaningful search query that captures what the user is looking for. The search query should never be empty.

Search query examples:
- For sleep with tinctures: "tinctures sleep aid insomnia cannabis products"
- For pain with oils: "oils pain relief cannabis products" 
- For anxiety with edibles: "edibles anxiety relief CBD products"
- For focus with vaping: "vape pens focus creativity cannabis products"
- For general tinctures: "tinctures cannabis products"
- For general oils: "oils cannabis products"
- For general browsing: "cannabis products general recommendations"
- For beginners: "beginner friendly cannabis products"

IMPORTANT: When generating search queries, ALWAYS preserve specific product types mentioned by the user. Use these cannabis consumption method categories:

CONSUMPTION METHODS TO RECOGNIZE:
- Inhalation: Smoking (joints, pipes, bongs), Vaporizing (dry herb, concentrates, cartridges)
- Oral: Edibles (gummies, chocolates, baked goods, beverages), Tinctures/Oils (sublingual or swallowed), Capsules
- Topical: Creams, balms, lotions, patches
- Other: Dabbing (concentrates), Suppositories

If the user mentions "tinctures for sleep", generate "tinctures sleep aid cannabis products", not just "sleep aid cannabis products".
If the user mentions "oils for pain", generate "oils pain relief cannabis products", not just "pain relief cannabis products".

Respond in JSON format with:
- should_show_recommendations: boolean
- recommendation_query: string (NEVER EMPTY if showing recommendations)
- reason: string (brief explanation of your decision)

Example response:
{
  "should_show_recommendations": true,
  "recommendation_query": "sleep aid insomnia cannabis products",
  "reason": "User mentioned having trouble sleeping and is looking for help"
}`

	// Get the last few messages for context (last 4 messages max)
	contextMessages := []string{}
	messageCount := 0
	for i := len(messages) - 1; i >= 0 && messageCount < 4; i-- {
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

	// Get the last user message for specific analysis
	lastUserMessage := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserMessage = messages[i].Content
			break
		}
	}

	conversationContext := strings.Join(contextMessages, "\n")

	// Prepare context-aware prompt
	prompt := fmt.Sprintf(`Please analyze this conversation and determine if product recommendations should be shown:

RECENT CONVERSATION:
%s

CURRENT USER MESSAGE: "%s"

Consider the conversation context. Is the user asking for product recommendations? This includes:
- Direct requests for products/recommendations
- Positive responses to AI asking if they want product suggestions ("yes", "sure", "yep", etc.)
- Expressing interest in cannabis products for their needs

Focus on whether they want to see products, even if they're responding to AI questions.`, conversationContext, lastUserMessage)

	log.Printf("Analyzing user message for recommendations: '%s'", lastUserMessage)
	log.Printf("Sending recommendation analysis request to OpenAI...")

	// Create the OpenAI request
	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   300,
	}

	// Debug log: Print the complete request being sent to OpenAI for product recommendation analysis
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

	resp, err := oc.client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Printf("ERROR: Failed to get product recommendation decision from OpenAI: %v", err)
		return nil, fmt.Errorf("failed to get product recommendation decision: %w", err)
	}

	if len(resp.Choices) == 0 {
		log.Printf("ERROR: No response choices from OpenAI for recommendation decision")
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// Parse the JSON response
	var decision ProductRecommendationDecision
	responseContent := resp.Choices[0].Message.Content
	log.Printf("OpenAI recommendation decision response: %s", responseContent)

	if err := json.Unmarshal([]byte(responseContent), &decision); err != nil {
		log.Printf("ERROR: Failed to parse product recommendation decision JSON: %v", err)
		log.Printf("Raw response content: %s", responseContent)
		// Return a default decision if JSON parsing fails
		return &ProductRecommendationDecision{
			ShouldShowRecommendations: false,
			RecommendationQuery:       "",
			Reason:                    "Failed to parse OpenAI response",
		}, nil
	}

	// flow-only logging handled by caller
	log.Printf("Should show recommendations: %t", decision.ShouldShowRecommendations)
	log.Printf("Recommendation query: '%s'", decision.RecommendationQuery)
	log.Printf("Reason: %s", decision.Reason)

	return &decision, nil
}

// GenerateEmbeddings generates embeddings for the given text using OpenAI's embedding model
func (oc *OpenAIClient) GenerateEmbeddings(ctx context.Context, text string) ([]float32, error) {
	req := openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.AdaEmbeddingV2,
	}

	resp, err := oc.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embeddings: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned from OpenAI")
	}

	return resp.Data[0].Embedding, nil
}

// GetStreamResponseWithKnowledge generates a streaming AI response enhanced with cannabis knowledge context
func (oc *OpenAIClient) GetStreamResponseWithKnowledge(ctx context.Context, messages []ChatMessage, knowledgeContext *KnowledgeResponse) (*openai.ChatCompletionStream, error) {
	return oc.GetStreamResponseWithKnowledgeAndSystemPrompt(ctx, messages, knowledgeContext, GetMainConversationalSystemPrompt())
}

// GetStreamResponseWithKnowledgeAndSystemPrompt generates a streaming AI response with custom system prompt and knowledge context
func (oc *OpenAIClient) GetStreamResponseWithKnowledgeAndSystemPrompt(ctx context.Context, messages []ChatMessage, knowledgeContext *KnowledgeResponse, systemPrompt string) (*openai.ChatCompletionStream, error) {
	log.Printf("Creating knowledge-enhanced AI stream response")
	log.Printf("Input messages: %d", len(messages))
	log.Printf("Knowledge context available: %t", knowledgeContext != nil && knowledgeContext.HasRelevantKnowledge)

	// Enhance messages with cannabis knowledge context
	enhancedMessages := oc.enhanceMessagesWithKnowledgeAndSystemPrompt(messages, knowledgeContext, systemPrompt)

	log.Printf("Enhanced messages count: %d", len(enhancedMessages))

	// Use the existing GetStreamResponseWithSystemPrompt method with enhanced messages
	return oc.GetStreamResponseWithSystemPrompt(ctx, enhancedMessages, systemPrompt)
}

// GetResponseWithKnowledge generates an AI response enhanced with cannabis knowledge context
func (oc *OpenAIClient) GetResponseWithKnowledge(ctx context.Context, messages []ChatMessage, knowledgeContext *KnowledgeResponse) (string, error) {
	log.Printf("=== Creating Knowledge-Enhanced AI Response ===")
	log.Printf("Input messages: %d", len(messages))
	log.Printf("Knowledge context available: %t", knowledgeContext != nil && knowledgeContext.HasRelevantKnowledge)

	// Enhance messages with cannabis knowledge context
	enhancedMessages := oc.enhanceMessagesWithKnowledge(messages, knowledgeContext)

	log.Printf("Enhanced messages count: %d", len(enhancedMessages))

	// Use the existing GetResponse method with enhanced messages
	return oc.GetResponse(ctx, enhancedMessages)
}

// enhanceMessagesWithKnowledge adds cannabis knowledge context to the conversation
func (oc *OpenAIClient) enhanceMessagesWithKnowledge(messages []ChatMessage, knowledgeContext *KnowledgeResponse) []ChatMessage {
	return oc.enhanceMessagesWithKnowledgeAndSystemPrompt(messages, knowledgeContext, GetMainConversationalSystemPrompt())
}

// ProductInsightsInput contains the product data needed to generate AI insights
type ProductInsightsInput struct {
	ProductID   int64   `json:"product_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	THCContent  float64 `json:"thc_content"`
	CBDContent  float64 `json:"cbd_content"`
	StrainType  string  `json:"strain_type"`
}

// ProductInsightsOutput contains AI-generated insights for a product
type ProductInsightsOutput struct {
	ProductID      int64    `json:"product_id"`
	WhyThisProduct string   `json:"why_this_product"`
	TopFeelings    []string `json:"top_feelings"`
	KeyInfo        KeyInfo  `json:"key_info"`
	Ingredients    string   `json:"ingredients"`
}

// KeyInfo represents structured key information about a product
type KeyInfo struct {
	Flavor            string `json:"flavor,omitempty"`
	THCCBDContent     string `json:"thc_cbd_content,omitempty"`
	StrainType        string `json:"strain_type,omitempty"`
	TypicalTimeOfUse  string `json:"typical_time_of_use,omitempty"`
	ExpectedIntensity string `json:"expected_intensity,omitempty"`
	ConsumptionFormat string `json:"consumption_format,omitempty"`
}

// GenerateProductInsights generates AI insights for multiple products
// Uses GPT-4.1 for best quality results, accuracy, and cannabis domain knowledge
func (oc *OpenAIClient) GenerateProductInsights(ctx context.Context, products []ProductInsightsInput) ([]ProductInsightsOutput, error) {
	if len(products) == 0 {
		return []ProductInsightsOutput{}, nil
	}

	log.Printf("=== Generating AI Insights for %d Products ===", len(products))

	var results []ProductInsightsOutput

	// Process products in batches to avoid token limits
	batchSize := 5
	for i := 0; i < len(products); i += batchSize {
		end := i + batchSize
		if end > len(products) {
			end = len(products)
		}
		batch := products[i:end]

		batchResults, err := oc.generateInsightsBatch(ctx, batch)
		if err != nil {
			log.Printf("ERROR: Failed to generate insights for batch %d-%d: %v", i, end, err)
			// Continue with other batches, don't fail entire operation
			continue
		}

		results = append(results, batchResults...)
	}

	log.Printf("Successfully generated insights for %d/%d products", len(results), len(products))
	return results, nil
}

// generateInsightsBatch generates insights for a batch of products
func (oc *OpenAIClient) generateInsightsBatch(ctx context.Context, products []ProductInsightsInput) ([]ProductInsightsOutput, error) {
	// Build product descriptions for the prompt
	var productDescriptions strings.Builder
	for i, p := range products {
		productDescriptions.WriteString(fmt.Sprintf(`
Product %d (ID: %d):
- Name: %s
- Description: %s
- Category: %s
- THC Content: %.1f%%
- CBD Content: %.1f%%
- Strain Type: %s
`, i+1, p.ProductID, p.Name, p.Description, p.Category, p.THCContent, p.CBDContent, p.StrainType))
	}

	systemPrompt := `You are a cannabis product expert assistant with deep knowledge of cannabis strains, effects, and products. Your task is to generate accurate, helpful insights about cannabis products.

KNOWLEDGE USAGE GUIDELINES:
1. USE PROVIDED DATA FIRST - Always prioritize the product information provided (name, description, THC/CBD content, strain type, category)
2. FILL GAPS WITH YOUR EXPERTISE - If specific fields are empty or missing in the provided data, you MAY use your cannabis expertise to provide accurate information, but ONLY if you are highly confident about the value
3. NEVER GUESS OR FABRICATE - If you are not confident about a value, return an empty string or empty array for that field. False information is worse than no information
4. BE HONEST ABOUT CONFIDENCE - Only provide information you would stake your reputation on as a cannabis expert
5. Use accurate cannabis terminology and effects based on known strain types (Indica, Sativa, Hybrid)

EXAMPLE: If a product named "Blue Dream" has empty THC content, you may provide typical THC range for Blue Dream (17-24%) since this is well-documented. But if it's an unknown product with no data, leave it empty.

For each product, generate:
1. why_this_product: 2-3 sentences about why and when this product is useful. Base this on product name, strain type, THC/CBD content, and your cannabis knowledge.
2. top_feelings: Array of 3-5 feelings/effects the user might experience. Use known effects for the strain type or specific strain if you recognize it.
3. key_info: Object with structured details:
   - flavor: From description OR your knowledge of this specific strain if you recognize it
   - thc_cbd_content: Format as "THC: X% | CBD: Y%" - use provided values or known typical ranges for recognized strains
   - strain_type: Indica/Sativa/Hybrid with brief effect note
   - typical_time_of_use: Based on strain type (Indica=evening/night, Sativa=day, Hybrid=any)
   - expected_intensity: Based on THC content (0-15%=light, 15-25%=moderate, 25%+=strong)
   - consumption_format: From description or category
4. ingredients: Include if mentioned in description, or provide common terpene profile if you recognize the strain. Leave empty if unknown.

Respond with a JSON array of objects matching the exact product order provided.`

	userPrompt := fmt.Sprintf(`Generate insights for these cannabis products. Return ONLY a valid JSON array with no additional text:

%s

Return format:
[
  {
    "product_id": <number>,
    "why_this_product": "<string>",
    "top_feelings": ["<string>", ...],
    "key_info": {
      "flavor": "<string or empty>",
      "thc_cbd_content": "<string>",
      "strain_type": "<string>",
      "typical_time_of_use": "<string>",
      "expected_intensity": "<string>",
      "consumption_format": "<string or empty>"
    },
    "ingredients": "<string or empty>"
  }
]`, productDescriptions.String())

	req := openai.ChatCompletionRequest{
		Model: openai.GPT4Dot1, // Using GPT-4.1 - latest stable model with best accuracy
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
		Temperature: 0.3, // Low temperature for consistent, factual responses
		MaxTokens:   2500,
	}

	log.Printf("Sending product insights request to OpenAI (GPT-4.1) for %d products", len(products))

	resp, err := oc.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion for product insights: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response choices returned for product insights")
	}

	responseContent := strings.TrimSpace(resp.Choices[0].Message.Content)
	log.Printf("Received product insights response (length: %d)", len(responseContent))

	// Clean up response - remove markdown code blocks if present
	responseContent = strings.TrimPrefix(responseContent, "```json")
	responseContent = strings.TrimPrefix(responseContent, "```")
	responseContent = strings.TrimSuffix(responseContent, "```")
	responseContent = strings.TrimSpace(responseContent)

	// Parse the JSON response
	var insights []ProductInsightsOutput
	if err := json.Unmarshal([]byte(responseContent), &insights); err != nil {
		log.Printf("ERROR: Failed to parse product insights JSON: %v", err)
		log.Printf("Raw response: %s", responseContent[:min(500, len(responseContent))])
		return nil, fmt.Errorf("failed to parse product insights response: %w", err)
	}

	// Validate product IDs match
	for i, insight := range insights {
		if i < len(products) && insight.ProductID == 0 {
			insights[i].ProductID = products[i].ProductID
		}
	}

	return insights, nil
}

// enhanceMessagesWithKnowledgeAndSystemPrompt adds cannabis knowledge context to the conversation with custom system prompt
func (oc *OpenAIClient) enhanceMessagesWithKnowledgeAndSystemPrompt(messages []ChatMessage, knowledgeContext *KnowledgeResponse, systemPrompt string) []ChatMessage {
	log.Printf("=== Enhancing Messages with Cannabis Knowledge ===")

	if knowledgeContext == nil {
		log.Printf("No knowledge context provided - using messages as-is")
		return messages
	}

	if !knowledgeContext.HasRelevantKnowledge {
		log.Printf("Knowledge context has no relevant knowledge (query: '%s') - using messages as-is", knowledgeContext.Query)
		return messages
	}

	log.Printf("Knowledge enhancement details:")
	log.Printf("- Knowledge query: '%s'", knowledgeContext.Query)
	log.Printf("- Knowledge results: %d", len(knowledgeContext.Results))
	log.Printf("- Context text length: %d characters", len(knowledgeContext.ContextText))

	// Log knowledge quality metrics
	if len(knowledgeContext.Results) > 0 {
		highQuality := 0
		mediumQuality := 0
		for _, result := range knowledgeContext.Results {
			if result.Score >= 0.8 {
				highQuality++
			} else if result.Score >= 0.7 {
				mediumQuality++
			}
		}
		log.Printf("- Knowledge quality: %d high (≥0.8), %d medium (≥0.7), %d total",
			highQuality, mediumQuality, len(knowledgeContext.Results))
	}

	// Create a knowledge context message to prepend
	knowledgeContent := fmt.Sprintf(`%s

## 📚 CANNABIS KNOWLEDGE BASE:
%s

## 🎯 Knowledge-Enhanced Response Guidelines:
• Use the knowledge base above to provide accurate, educational information
• Keep responses conversational and concise (following your bee persona)
• If the knowledge base doesn't have specific info, say "I don't have details on that"
• **ALWAYS prioritize cannabis effects and benefits FIRST** when discussing products
• **PRESERVE SPECIFIC CONSUMPTION METHODS** - if user mentions tinctures, oils, edibles, etc., maintain that specificity
• **Secondary ingredients** (ginger, peppermint, etc.) should be mentioned as supplementary context only
• **Clear separation**: Maintain distinction between cannabis effects vs. other ingredients
• Focus on education, effects, safety, and general guidance
• NEVER recommend specific products, brands, or retailers in your text response
• Let the separate product recommendation system handle showing actual products
• Avoid lengthy medical disclaimers - keep it brief and friendly

Remember: You're Cannabee 🐝 - stay true to your conversational, helpful personality while providing knowledge-based education! Cannabis effects come FIRST, other ingredients are secondary! PRESERVE the exact consumption method the user mentioned!`, systemPrompt, knowledgeContext.ContextText)

	knowledgeMessage := ChatMessage{
		Role:    "system",
		Content: knowledgeContent,
	}

	// Prepend knowledge context message to the conversation
	enhancedMessages := make([]ChatMessage, 0, len(messages)+1)
	enhancedMessages = append(enhancedMessages, knowledgeMessage)
	enhancedMessages = append(enhancedMessages, messages...)

	log.Printf("Knowledge enhancement complete:")
	log.Printf("- Original messages: %d", len(messages))
	log.Printf("- Knowledge context message added: 1")
	log.Printf("- Total enhanced messages: %d", len(enhancedMessages))
	log.Printf("- Knowledge context preview: %.200s...", knowledgeContext.ContextText)

	return enhancedMessages
}
