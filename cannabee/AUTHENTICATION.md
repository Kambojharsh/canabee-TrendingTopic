# JWT Authentication with AWS Cognito

This Go service now implements JWT authentication using AWS Cognito. All API endpoints (except `/health`) require valid JWT tokens for access.

## Environment Variables

You need to set the following environment variables:

```bash
# Required for JWT authentication
export AWS_COGNITO_USER_POOL_ID="your-user-pool-id"
export AWS_REGION="us-east-1"  # or your AWS region

# Required for OpenAI functionality
export OPENAI_API_KEY="your-openai-api-key"

# Database configuration (if not already set)
export DB_HOST="localhost"
export DB_PORT="3306"
export DB_USER="username"
export DB_PASSWORD="password"
export DB_NAME="database_name"
```

## How Authentication Works

### 1. JWT Token Validation
- The service fetches AWS Cognito's JSON Web Key Set (JWKS) to validate JWT signatures
- Tokens are validated for:
  - Valid signature using Cognito's public keys
  - Correct issuer (your Cognito User Pool)
  - Not expired
  - Valid token use (`access` or `id`)

### 2. HTTP API Authentication
All protected endpoints require the `Authorization` header:

```bash
Authorization: Bearer <your-jwt-token>
```

Example:
```bash
curl -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." \
     http://localhost:8080/sessions
```

### 3. WebSocket Authentication
WebSocket connections require the JWT token as a query parameter since WebSockets don't support custom headers during the upgrade:

```
ws://localhost:8080/chat/{sessionId}?token=<your-jwt-token>
```

## API Endpoints

### Public Endpoints (No Authentication)
- `GET /health` - Health check

### Protected Endpoints (JWT Required)
- `GET /sessions` - Get user's chat sessions
- `POST /sessions` - Create new chat session
- `GET /sessions/{sessionId}` - Get specific session details
- `DELETE /sessions/{sessionId}` - Delete session
- `GET /sessions/{sessionId}/summary` - Get session summary
- `GET /sessions/{sessionId}/tags` - Get session tags
- `GET /sessions/{sessionId}/recommendations` - Get similar sessions
- `GET /analytics` - Get user session analytics
- `GET /tags/popular` - Get popular tags
- `GET /chat/{sessionId}/messages` - Get chat messages
- `GET /chat/{sessionId}?token=JWT` - WebSocket endpoint

## User Context

The authenticated user information is automatically extracted from the JWT token:
- User ID (`cognito:username` claim)
- Email address (`email` claim)
- Cognito Subject (`sub` claim)

This information is available in the request context for all protected endpoints.

## Changes Made

### 1. Authentication System (`pkg/auth/auth.go`)
- Replaced simple session tokens with JWT validation
- Added AWS Cognito integration
- Implemented JWKS caching for performance
- Added middleware for HTTP authentication
- Added WebSocket token validation

### 2. HTTP Handlers (`pkg/handlers/handlers.go`)
- Updated all handlers to extract user ID from JWT context instead of query parameters
- Removed `user_id` parameter requirements
- Added proper authentication error handling

### 3. WebSocket Handler
- Added JWT token validation for WebSocket connections
- Token passed via query parameter: `?token=<jwt>`
- User information extracted from validated token

### 4. Main Server (`cmd/server/main.go`)
- Added authentication middleware to all protected routes
- Health endpoint remains public
- Updated logging to show authentication requirements

## Error Responses

### 401 Unauthorized
- Missing Authorization header
- Invalid JWT token format
- Expired JWT token
- Invalid signature
- Wrong issuer

### 403 Forbidden
- User trying to access another user's resources

## Security Notes

1. **JWKS Caching**: The service caches Cognito's JWKS for 24 hours to improve performance
2. **Token Validation**: All JWT tokens are validated against Cognito's public keys
3. **User Isolation**: Users can only access their own sessions and data
4. **WebSocket Security**: Even WebSocket connections require valid JWT tokens

## Migration Notes

If you're migrating from the previous version:

1. **No more user_id parameters**: User ID is now extracted from JWT tokens
2. **Authentication required**: All endpoints except `/health` now require valid JWT tokens
3. **WebSocket changes**: WebSocket connections now require `?token=JWT` instead of `?user_id=X`

## Testing

You can test the authentication by:

1. Getting a JWT token from your Cognito User Pool
2. Making requests with the token:

```bash
# Test health endpoint (no auth)
curl http://localhost:8080/health

# Test protected endpoint (with auth)
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
     http://localhost:8080/sessions

# Test WebSocket (replace YOUR_JWT_TOKEN and SESSION_ID)
wscat -c "ws://localhost:8080/chat/SESSION_ID?token=YOUR_JWT_TOKEN"
```

## Troubleshooting

### "AWS_COGNITO_USER_POOL_ID environment variable is required"
Set the `AWS_COGNITO_USER_POOL_ID` environment variable with your Cognito User Pool ID.

### "failed to fetch JWKS"
Check your `AWS_REGION` environment variable and ensure the server has internet access to reach Cognito's JWKS endpoint.

### "Invalid or expired token"
Ensure your JWT token is:
- Not expired
- Issued by the correct Cognito User Pool
- Has the correct format (`Bearer <token>`)

### WebSocket connection issues
Make sure you're passing the token as a query parameter: `?token=<jwt>` not in headers.
