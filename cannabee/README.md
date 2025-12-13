# Cannabee API

A real-time chat API service with AI-powered responses, session management, and intelligent recommendations.

## Features

### Core Features

- Real-time WebSocket chat with AI responses
- Session management with user tracking
- OpenAI integration for AI responses
- Database storage for chat history

### New Features (Session Intelligence)

- **Automatic Session Completion**: When a session ends, the system automatically:

  - Generates a concise summary of the conversation
  - Extracts relevant tags/keywords using AI
  - Stores both in the database for future use

- **Context-Aware Conversations**: 🧠 **NEW**

  - **Personalized Greetings**: Based on user's last 3 sessions (e.g., "Welcome back! How has your pain management been going?")
  - **Contextual AI Responses**: AI considers previous conversations for more relevant answers
  - **Stack-like Tag Management**: Automatically keeps only the last 3 sessions' tags, removes older ones
  - **Lazy Cleanup**: Old session tags are cleaned up when new sessions start

- **Session Recommendations**: Find similar conversations based on shared tags
- **Analytics**: Get insights into user behavior and popular topics
- **Smart Tag Management**: Normalized tag storage with automatic cleanup

## API Endpoints

### Basic Session Management

- `GET /health` - Health check
- `GET /sessions?user_id=X` - Get user's chat sessions
- `POST /sessions` - Create new chat session
- `GET /sessions/{sessionId}` - Get specific session details
- `GET /chat/{sessionId}?user_id=X` - WebSocket endpoint for chat

### Session Intelligence & Recommendations

- `GET /sessions/{sessionId}/summary` - Get AI-generated session summary
- `GET /sessions/{sessionId}/tags` - Get session tags/keywords
- `GET /sessions/{sessionId}/recommendations?limit=10` - Get similar sessions
- `GET /analytics?user_id=X` - Get user session analytics
- `GET /tags/popular?limit=20` - Get popular tags across all sessions

## Database Schema

### Core Tables

- `users` - User information
- `sessions` - Chat sessions with metadata
- `chats` - Individual chat messages
- `roles` - User roles and permissions

### New Intelligence Tables

- `session_summaries` - AI-generated session summaries
- `session_tags` - Normalized tags for each session

## How Session Completion Works

1. **Trigger**: When a WebSocket client disconnects (session ends)
2. **Analysis**: System checks if session has meaningful conversation (3+ messages)
3. **AI Processing**:
   - Generates concise summary using OpenAI
   - Extracts 3-8 relevant tags/keywords
   - Normalizes tags to lowercase
4. **Storage**: Saves summary and tags to database
5. **Session Update**: Marks session as inactive

## Context-Aware System

### How Personalized Greetings Work

1. **New Session Start**: When a user starts a new session
2. **Context Retrieval**: System fetches summaries and tags from user's last 3 sessions
3. **AI Generation**: OpenAI generates personalized greeting based on previous topics
4. **Fallback Logic**: Built-in patterns for common topics (pain, sleep, anxiety, etc.)
5. **Tag Cleanup**: Lazily removes tags from sessions older than last 3

### Example Greeting Progression

```
First session: "Hello! I'm your cannabis assistant..."
After pain discussion: "Welcome back! How has your pain management been going?"
After dosage questions: "Hello again! How did those dosage recommendations work out?"
```

### Enhanced AI Responses

- AI receives context from previous sessions as system message
- Responses reference past conversations when relevant
- Maintains continuity across multiple sessions

## Setup & Installation

1. **Database Setup**:

   ```bash
   # Run the schema files in order
   mysql -u root -p cannabee < pkg/db/schema/000_roles.sql
   mysql -u root -p cannabee < pkg/db/schema/001_users.sql
   mysql -u root -p cannabee < pkg/db/schema/002_sessions.sql
   mysql -u root -p cannabee < pkg/db/schema/003_chats.sql
   mysql -u root -p cannabee < pkg/db/schema/004_session_summaries.sql
   mysql -u root -p cannabee < pkg/db/schema/005_session_tags.sql
   ```

2. **Environment Variables**:

   ```bash
   export OPENAI_API_KEY="your-openai-api-key"
   export DB_USER="your-db-user"
   export DB_PASSWORD="your-db-password"
   export DB_HOST="localhost"
   export DB_PORT="3306"
   export DB_NAME="cannabee"
   ```

3. **Run the Server**:
   ```bash
   go run cmd/server/main.go
   ```

## Example Usage

### 1. Create and Use a Chat Session

```bash
# Create session
curl -X POST http://localhost:8080/sessions \
  -d '{"user_id":"user123","metadata":"Cannabis consultation"}'

# Connect via WebSocket and chat
# ws://localhost:8080/chat/{sessionId}?user_id=user123

# After session ends, get summary and tags
curl http://localhost:8080/sessions/{sessionId}/summary
curl http://localhost:8080/sessions/{sessionId}/tags
```

### 2. Get Recommendations

```bash
# Find similar sessions
curl http://localhost:8080/sessions/{sessionId}/recommendations?limit=5

# Get user analytics
curl http://localhost:8080/analytics?user_id=user123

# See popular topics
curl http://localhost:8080/tags/popular?limit=10
```

### 3. Experience Context-Aware Features

The following features work automatically when you use the chat:

- **Personalized Greetings**: Automatically generated when starting a new session
- **Context-Enhanced Responses**: AI automatically considers your previous 3 sessions
- **Tag Cleanup**: Old session tags are automatically cleaned up to keep only the last 3

## Response Examples

### Session Summary

```json
{
  "id": 1,
  "session_id": "123e4567-e89b-12d3-a456-426614174000",
  "summary": "User asked about CBD oil dosage for anxiety. Discussed starting with 5-10mg daily and gradually increasing. Recommended consulting healthcare provider.",
  "created_at": "2024-01-15T10:30:00Z"
}
```

### Session Tags

```json
[
  {
    "id": 1,
    "session_id": "123e4567-e89b-12d3-a456-426614174000",
    "tag": "cbd",
    "created_at": "2024-01-15T10:30:00Z"
  },
  {
    "id": 2,
    "session_id": "123e4567-e89b-12d3-a456-426614174000",
    "tag": "anxiety",
    "created_at": "2024-01-15T10:30:00Z"
  }
]
```

### Recommendations

```json
[
  {
    "session_id": "456e7890-e89b-12d3-a456-426614174001",
    "summary": "Discussion about CBD dosing for sleep issues",
    "matching_tags": ["cbd", "dosage"],
    "similarity_score": 2
  }
]
```

## Technology Stack

- **Backend**: Go with Gorilla WebSocket and Mux
- **Database**: MySQL with SQLC for type-safe queries
- **AI**: OpenAI GPT-3.5-turbo for chat and analysis
- **Real-time**: WebSocket connections for live chat
- **HTTP**: RESTful API for session and analytics management
