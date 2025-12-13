package auth

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// GuestCredentials represents the AWS credentials provided by the frontend
type GuestCredentials struct {
	AccessKey    string `json:"access_key"`
	SecretKey    string `json:"secret_key"`
	SessionToken string `json:"session_token"`
}

// GuestValidationResponse contains the validated guest identity information
type GuestValidationResponse struct {
	Arn        string `json:"arn"`
	Account    string `json:"account"`
	UserId     string `json:"user_id"`
	IsValid    bool   `json:"is_valid"`
	IdentityID string `json:"identity_id"` // Cognito Identity ID extracted from ARN
}

// ValidateGuestCredentials validates AWS temporary credentials using STS GetCallerIdentity
// and ensures they come from the authorized Cognito Identity Pool
func ValidateGuestCredentials(ctx context.Context, accessKey, secretKey, sessionToken string) (*GuestValidationResponse, error) {
	log.Printf("=== Validating Guest Credentials ===")

	// CRITICAL: Session token is REQUIRED for temporary credentials
	// This prevents permanent IAM credentials from being accepted
	if sessionToken == "" {
		log.Printf("ERROR: Session token is required for guest access")
		return nil, fmt.Errorf("session token is required for temporary credentials")
	}

	// Create AWS config with provided credentials
	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion("us-east-1"), // 👈 add your desired AWS region here
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				accessKey,
				secretKey,
				sessionToken,
			),
		),
	)
	if err != nil {
		log.Printf("ERROR: Failed to load AWS config: %v", err)
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Create STS client
	stsClient := sts.NewFromConfig(cfg)

	// Call GetCallerIdentity to validate credentials
	// This will fail if credentials are invalid or expired
	resp, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		log.Printf("ERROR: Invalid or expired credentials: %v", err)
		return nil, fmt.Errorf("invalid credentials: %w", err)
	}

	// Extract information from response
	arn := aws.ToString(resp.Arn)
	account := aws.ToString(resp.Account)
	userId := aws.ToString(resp.UserId)

	log.Printf("Caller ARN: %s", arn)
	log.Printf("Account: %s", account)
	log.Printf("UserId: %s", userId)

	// CRITICAL: Verify that credentials are temporary (assumed-role)
	// This ensures permanent IAM user credentials are rejected
	if !strings.Contains(arn, "assumed-role") {
		log.Printf("ERROR: Credentials must be temporary (assumed-role), not permanent IAM credentials")
		return nil, fmt.Errorf("only temporary credentials from Cognito Identity Pool are allowed")
	}

	// Validate that credentials come from authorized Cognito Identity Pool
	if err := validateCognitoIdentityPool(arn, account); err != nil {
		log.Printf("ERROR: Unauthorized credentials: %v", err)
		return nil, fmt.Errorf("unauthorized: %w", err)
	}

	// Extract Cognito Identity ID from ARN
	// ARN format: arn:aws:sts::ACCOUNT:assumed-role/ROLE_NAME/IDENTITY_ID
	identityID := extractIdentityIDFromARN(arn)
	if identityID != "" {
		log.Printf("Extracted Cognito Identity ID: %s", identityID)
	}

	log.Printf("✅ Guest credentials validated successfully")

	return &GuestValidationResponse{
		Arn:        arn,
		Account:    account,
		UserId:     userId,
		IsValid:    true,
		IdentityID: identityID,
	}, nil
}

// validateCognitoIdentityPool ensures credentials come from the authorized Identity Pool
func validateCognitoIdentityPool(arn, account string) error {
	// Get expected values from environment
	cognitoRoleName := os.Getenv("COGNITO_UNAUTHENTICATED_ROLE_NAME")
        expectedAccount := os.Getenv("AWS_ACCOUNT_ID")
	// // If no validation configured, allow (for development)
	// if expectedAccount == "" && identityPoolID == "" {
	// 	log.Printf("WARNING: No Identity Pool validation configured (AWS_ACCOUNT_ID and COGNITO_IDENTITY_POOL_ID not set)")
	// 	return nil
	// 
	//}
        log.Printf("expected AWS_ACCOUNT_ID %s",expectedAccount)
	log.Printf("actual acc id %s",account)
	// Validate AWS account
	if expectedAccount != "" && account != expectedAccount {
		return fmt.Errorf("credentials from unauthorized AWS account: %s (expected: %s)", account, expectedAccount)
	}

	// Validate role name if configured
	// Expected ARN format: arn:aws:sts::ACCOUNT:assumed-role/Cognito_ROLE_NAME/IDENTITY_ID
	if cognitoRoleName != "" {
		expectedRoleArn := fmt.Sprintf("assumed-role/%s/", cognitoRoleName)
		if !strings.Contains(arn, expectedRoleArn) {
			return fmt.Errorf("credentials not from authorized Cognito role (expected role: %s)", cognitoRoleName)
		}
	}


	log.Printf("✅ Credentials validated against authorized Identity Pool")
	return nil
}

// extractIdentityIDFromARN extracts the Cognito Identity ID from the STS ARN
// ARN format: arn:aws:sts::ACCOUNT:assumed-role/ROLE_NAME/IDENTITY_ID
func extractIdentityIDFromARN(arn string) string {
	// Simple extraction - the identity ID is typically the last part after the last '/'
	for i := len(arn) - 1; i >= 0; i-- {
		if arn[i] == '/' {
			return arn[i+1:]
		}
	}
	return ""
}

// ValidateGuestCredentialsSimple is a simpler version that just returns error or nil
func ValidateGuestCredentialsSimple(ctx context.Context, accessKey, secretKey, sessionToken string) error {
	_, err := ValidateGuestCredentials(ctx, accessKey, secretKey, sessionToken)
	return err
}
