package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"database/sql"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserIDKey contextKey = "userID"
const UserEmailKey contextKey = "userEmail"
const CognitoSubKey contextKey = "cognitoSub"
const DatabaseUserIDKey contextKey = "databaseUserID" // New key for database user ID

// CognitoJWTClaims represents the custom claims in Cognito JWT tokens
type CognitoJWTClaims struct {
	Aud           interface{} `json:"aud"`
	Iss           string      `json:"iss"`
	Sub           string      `json:"sub"`
	Email         string      `json:"email"`
	EmailVerified bool        `json:"email_verified"`
	TokenUse      string      `json:"token_use"`
	AuthTime      int64       `json:"auth_time"`
	Exp           int64       `json:"exp"`
	Iat           int64       `json:"iat"`
	Username      string      `json:"cognito:username"`
	jwt.RegisteredClaims
}

// JWK represents a JSON Web Key
type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS represents a JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// CognitoAuth handles JWT authentication with AWS Cognito
type CognitoAuth struct {
	userPoolID   string
	region       string
	jwksURL      string
	cachedJWKS   *JWKS
	lastFetched  time.Time
	cacheTimeout time.Duration
}

// NewCognitoAuth creates a new CognitoAuth instance
func NewCognitoAuth() (*CognitoAuth, error) {
	userPoolID := os.Getenv("AWS_COGNITO_USER_POOL_ID")
	region := os.Getenv("AWS_REGION")

	if userPoolID == "" {
		return nil, errors.New("AWS_COGNITO_USER_POOL_ID environment variable is required")
	}
	if region == "" {
		return nil, errors.New("AWS_REGION environment variable is required")
	}

	jwksURL := fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s/.well-known/jwks.json", region, userPoolID)

	log.Printf("🔐 [AUTH] Initializing Cognito JWT authentication")
	log.Printf("🔐 [AUTH] User Pool ID: %s", userPoolID)
	log.Printf("🔐 [AUTH] Region: %s", region)
	log.Printf("🔐 [AUTH] JWKS URL: %s", jwksURL)

	return &CognitoAuth{
		userPoolID:   userPoolID,
		region:       region,
		jwksURL:      jwksURL,
		cacheTimeout: 24 * time.Hour, // Cache JWKS for 24 hours
	}, nil
}

// fetchJWKS fetches the JSON Web Key Set from Cognito
func (ca *CognitoAuth) fetchJWKS() (*JWKS, error) {
	// Check if we have cached JWKS that's still valid
	if ca.cachedJWKS != nil && time.Since(ca.lastFetched) < ca.cacheTimeout {
		log.Printf("🔐 [JWKS] Using cached JWKS (age: %v)", time.Since(ca.lastFetched))
		return ca.cachedJWKS, nil
	}

	log.Printf("🔐 [JWKS] Fetching fresh JWKS from: %s", ca.jwksURL)

	resp, err := http.Get(ca.jwksURL)
	if err != nil {
		log.Printf("❌ [JWKS] Failed to fetch JWKS: %v", err)
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ [JWKS] Failed to fetch JWKS: status %d", resp.StatusCode)
		return nil, fmt.Errorf("failed to fetch JWKS: status %d", resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		log.Printf("❌ [JWKS] Failed to decode JWKS: %v", err)
		return nil, fmt.Errorf("failed to decode JWKS: %w", err)
	}

	// Cache the JWKS
	ca.cachedJWKS = &jwks
	ca.lastFetched = time.Now()

	log.Printf("✅ [JWKS] Successfully fetched and cached JWKS with %d keys", len(jwks.Keys))
	return &jwks, nil
}

// getKey returns the RSA public key for the given key ID
func (ca *CognitoAuth) getKey(kid string) (*rsa.PublicKey, error) {
	log.Printf("🔐 [KEY] Looking for key with ID: %s", kid)

	jwks, err := ca.fetchJWKS()
	if err != nil {
		return nil, err
	}

	for _, key := range jwks.Keys {
		if key.Kid == kid && key.Kty == "RSA" {
			log.Printf("✅ [KEY] Found RSA key with ID: %s", kid)
			return ca.convertJWKToRSAPublicKey(key)
		}
	}

	log.Printf("❌ [KEY] Key with ID %s not found in JWKS", kid)
	return nil, fmt.Errorf("key with ID %s not found", kid)
}

// convertJWKToRSAPublicKey converts a JWK to an RSA public key
func (ca *CognitoAuth) convertJWKToRSAPublicKey(jwk JWK) (*rsa.PublicKey, error) {
	log.Printf("🔐 [KEY] Converting JWK to RSA public key for key ID: %s", jwk.Kid)

	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		log.Printf("❌ [KEY] Failed to decode 'n' parameter: %v", err)
		return nil, fmt.Errorf("failed to decode n: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		log.Printf("❌ [KEY] Failed to decode 'e' parameter: %v", err)
		return nil, fmt.Errorf("failed to decode e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())

	log.Printf("✅ [KEY] Successfully converted JWK to RSA public key")
	return &rsa.PublicKey{
		N: n,
		E: e,
	}, nil
}

// ValidateToken validates a JWT token and returns the claims
func (ca *CognitoAuth) ValidateToken(tokenString string) (*CognitoJWTClaims, error) {
	log.Printf("🔐 [TOKEN] Starting JWT token validation")
	log.Printf("🔐 [TOKEN] Token length: %d characters", len(tokenString))

	// Parse the token without validation first to get the header
	token, err := jwt.ParseWithClaims(tokenString, &CognitoJWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		log.Printf("🔐 [TOKEN] Parsing token with algorithm: %v", token.Method.Alg())

		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			log.Printf("❌ [TOKEN] Unexpected signing method: %v", token.Header["alg"])
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Get the key ID from the token header
		kid, ok := token.Header["kid"].(string)
		if !ok {
			log.Printf("❌ [TOKEN] 'kid' header missing from token")
			return nil, errors.New("kid header missing")
		}

		log.Printf("🔐 [TOKEN] Token key ID: %s", kid)

		// Get the public key for verification
		return ca.getKey(kid)
	})

	if err != nil {
		log.Printf("❌ [TOKEN] Failed to parse/validate token: %v", err)
		return nil, fmt.Errorf("failed to parse/validate token: %w", err)
	}

	// Check if token is valid
	if !token.Valid {
		log.Printf("❌ [TOKEN] Token is invalid")
		return nil, errors.New("token is invalid")
	}

	// Extract claims
	claims, ok := token.Claims.(*CognitoJWTClaims)
	if !ok {
		log.Printf("❌ [TOKEN] Invalid claims type")
		return nil, errors.New("invalid claims type")
	}

	log.Printf("🔐 [TOKEN] Token claims extracted successfully")
	log.Printf("🔐 [TOKEN] Subject (sub): %s", claims.Sub)
	log.Printf("🔐 [TOKEN] Username: %s", claims.Username)
	log.Printf("🔐 [TOKEN] Email: %s", claims.Email)
	log.Printf("🔐 [TOKEN] Token use: %s", claims.TokenUse)
	log.Printf("🔐 [TOKEN] Issuer: %s", claims.Iss)
	log.Printf("🔐 [TOKEN] Issued at: %v", time.Unix(claims.Iat, 0))
	log.Printf("🔐 [TOKEN] Expires at: %v", time.Unix(claims.Exp, 0))
	log.Printf("🔐 [TOKEN] Auth time: %v", time.Unix(claims.AuthTime, 0))

	// Validate issuer
	expectedIssuer := fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", ca.region, ca.userPoolID)
	if claims.Iss != expectedIssuer {
		log.Printf("❌ [TOKEN] Invalid issuer: expected %s, got %s", expectedIssuer, claims.Iss)
		return nil, fmt.Errorf("invalid issuer: expected %s, got %s", expectedIssuer, claims.Iss)
	}
	log.Printf("✅ [TOKEN] Issuer validation passed")

	// Validate token use (should be "access" or "id")
	if claims.TokenUse != "access" && claims.TokenUse != "id" {
		log.Printf("❌ [TOKEN] Invalid token use: %s", claims.TokenUse)
		return nil, fmt.Errorf("invalid token use: %s", claims.TokenUse)
	}
	log.Printf("✅ [TOKEN] Token use validation passed")

	// Validate expiration
	expirationTime := time.Unix(claims.Exp, 0)
	currentTime := time.Now()
	if expirationTime.Before(currentTime) {
		log.Printf("❌ [TOKEN] Token is expired: expired at %v, current time %v", expirationTime, currentTime)
		return nil, errors.New("token is expired")
	}
	log.Printf("✅ [TOKEN] Token expiration validation passed (expires in %v)", expirationTime.Sub(currentTime))

	log.Printf("✅ [TOKEN] Successfully validated JWT token for user: %s", claims.Sub)
	return claims, nil
}

// AuthMiddleware is the middleware function for JWT authentication
func (ca *CognitoAuth) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		log.Printf("🔐 [MIDDLEWARE] Processing request: %s %s", r.Method, r.URL.Path)
		log.Printf("🔐 [MIDDLEWARE] Remote address: %s", r.RemoteAddr)
		log.Printf("🔐 [MIDDLEWARE] User agent: %s", r.UserAgent())
		log.Printf("🔐 [MIDDLEWARE] Origin: %s", r.Header.Get("Origin"))
		log.Printf("🔐 [MIDDLEWARE] All headers: %v", r.Header)

		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Printf("❌ [MIDDLEWARE] Authorization header missing")
			// Add CORS headers even for unauthorized requests
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		log.Printf("🔐 [MIDDLEWARE] Authorization header present: %s", authHeader[:min(len(authHeader), 20)]+"...")

		// Expected format: "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			log.Printf("❌ [MIDDLEWARE] Invalid authorization header format: %s", authHeader)
			// Add CORS headers even for unauthorized requests
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			http.Error(w, "Invalid authorization header format. Expected: Bearer <token>", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		log.Printf("🔐 [MIDDLEWARE] Extracted token (length: %d)", len(tokenString))

		// Validate the JWT token
		log.Printf("🔐 [MIDDLEWARE] Starting JWT validation")
		claims, err := ca.ValidateToken(tokenString)
		if err != nil {
			log.Printf("❌ [MIDDLEWARE] JWT validation failed: %v", err)
			// Add CORS headers even for unauthorized requests
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		log.Printf("✅ [MIDDLEWARE] JWT validation successful")

		// Add user information to request context
		ctx := context.WithValue(r.Context(), UserIDKey, claims.Username) // Cognito username
		ctx = context.WithValue(ctx, UserEmailKey, claims.Email)          // User email
		ctx = context.WithValue(ctx, CognitoSubKey, claims.Sub)           // Cognito sub
		ctx = context.WithValue(ctx, DatabaseUserIDKey, "")               // Will be populated by handlers

		log.Printf("🔐 [MIDDLEWARE] User context injected:")
		log.Printf("   - Cognito Username: %s", claims.Username)
		log.Printf("   - Email: %s", claims.Email)
		log.Printf("   - Cognito Sub: %s", claims.Sub)
		log.Printf("   - Database User ID: (to be looked up)")

		// Debug: Log all available claims
		log.Printf("🔐 [DEBUG] All JWT claims:")
		log.Printf("   - Aud: %v", claims.Aud)
		log.Printf("   - Iss: %s", claims.Iss)
		log.Printf("   - Sub: %s", claims.Sub)
		log.Printf("   - Email: %s", claims.Email)
		log.Printf("   - EmailVerified: %t", claims.EmailVerified)
		log.Printf("   - TokenUse: %s", claims.TokenUse)
		log.Printf("   - AuthTime: %d", claims.AuthTime)
		log.Printf("   - Exp: %d", claims.Exp)
		log.Printf("   - Iat: %d", claims.Iat)
		log.Printf("   - Username: %s", claims.Username)

		// Continue with the request
		log.Printf("🔐 [MIDDLEWARE] Proceeding to handler")
		next.ServeHTTP(w, r.WithContext(ctx))

		processingTime := time.Since(startTime)
		log.Printf("✅ [MIDDLEWARE] Request processed successfully in %v", processingTime)
	})
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetUserIDFromContext extracts the user ID from the request context
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	if ok {
		log.Printf("🔐 [CONTEXT] Extracted user ID from context: %s", userID)
	} else {
		log.Printf("⚠️ [CONTEXT] User ID not found in context")
	}
	return userID, ok
}

// GetUserEmailFromContext extracts the user email from the request context
func GetUserEmailFromContext(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(UserEmailKey).(string)
	if ok {
		log.Printf("🔐 [CONTEXT] Extracted user email from context: %s", email)
	} else {
		log.Printf("⚠️ [CONTEXT] User email not found in context")
	}
	return email, ok
}

// GetCognitoSubFromContext extracts the Cognito sub from the request context
func GetCognitoSubFromContext(ctx context.Context) (string, bool) {
	sub, ok := ctx.Value(CognitoSubKey).(string)
	if ok {
		log.Printf("🔐 [CONTEXT] Extracted Cognito sub from context: %s", sub)
	} else {
		log.Printf("⚠️ [CONTEXT] Cognito sub not found in context")
	}
	return sub, ok
}

// GetDatabaseUserIDFromContext extracts the database user ID from the request context
func GetDatabaseUserIDFromContext(ctx context.Context) (string, bool) {
	databaseUserID, ok := ctx.Value(DatabaseUserIDKey).(string)
	if ok && databaseUserID != "" {
		log.Printf("🔐 [CONTEXT] Extracted database user ID from context: %s", databaseUserID)
	} else {
		log.Printf("⚠️ [CONTEXT] Database user ID not found in context")
	}
	return databaseUserID, ok
}

// SetDatabaseUserIDInContext sets the database user ID in the request context
func SetDatabaseUserIDInContext(ctx context.Context, databaseUserID string) context.Context {
	log.Printf("🔐 [CONTEXT] Setting database user ID in context: %s", databaseUserID)
	return context.WithValue(ctx, DatabaseUserIDKey, databaseUserID)
}

// LookupDatabaseUserID looks up the database user ID based on Cognito user ID
// This function should be called by handlers that have database access
func LookupDatabaseUserID(ctx context.Context, db *sql.DB, cognitoUserID string) (string, error) {
	log.Printf("🔐 [LOOKUP] Looking up database user ID for Cognito user: %s", cognitoUserID)

	// Get the email from the context to use for lookup
	// Since the users table doesn't have cognito_user_id, we'll use email
	email, ok := GetUserEmailFromContext(ctx)
	if !ok {
		log.Printf("⚠️ [LOOKUP] No email found in context for Cognito user: %s", cognitoUserID)
		return "", fmt.Errorf("email not found in context")
	}

	log.Printf("🔐 [LOOKUP] Looking up user by email: %s", email)

	// Query the users table to find the user by email
	query := `SELECT user_id FROM users WHERE email = ? LIMIT 1`

	var databaseUserID string
	err := db.QueryRowContext(ctx, query, email).Scan(&databaseUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("⚠️ [LOOKUP] No database user found for email: %s", email)
			return "", fmt.Errorf("user not found in database for email: %s", email)
		}
		log.Printf("❌ [LOOKUP] Database error looking up user: %v", err)
		return "", fmt.Errorf("database error: %w", err)
	}

	log.Printf("✅ [LOOKUP] Found database user ID: %s for email: %s", databaseUserID, email)
	return databaseUserID, nil
}

// ValidateWebSocketToken validates a JWT token for WebSocket connections
// This is a helper function that can be used in WebSocket upgrade handlers
func (ca *CognitoAuth) ValidateWebSocketToken(tokenString string) (*CognitoJWTClaims, error) {
	log.Printf("🔐 [WEBSOCKET] Validating JWT token for WebSocket connection")
	return ca.ValidateToken(tokenString)
}

// HealthCheckHandler is a simple health check that doesn't require authentication
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("🏥 [HEALTH] Health check request from %s", r.RemoteAddr)
	log.Printf("🏥 [HEALTH] Origin: %s", r.Header.Get("Origin"))

	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	log.Printf("✅ [HEALTH] Health check response sent")
}
