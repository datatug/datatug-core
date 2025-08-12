package gauth

import (
	"context"
	"fmt"
	"github.com/strongo/logus"
	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudresourcemanager/v3"
	"log"
	"net/http"
	"time"
)

const (
	keyringService = "datatug-app"
	keyringUser    = "google-oauth-refresh-token"
)

// getGoogleCloudClient handles the OAuth2 flow for desktop apps and caches the token locally.
func getGoogleCloudClient(ctx context.Context) (client *http.Client, err error) {

	// Cloud Resource Manager v3 scope
	// Use "Desktop app" type so no client secret is needed.
	config := &oauth2.Config{
		ClientID:     "588648831063-393c7c5gfj70sstaioked6qpb0sfj87h.apps.googleusercontent.com", // os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
		ClientSecret: "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",                                      // Creation date: 11 August 2025 at 16:03:21 GMT+1
		Scopes: []string{
			cloudresourcemanager.CloudPlatformReadOnlyScope,
		},
		Endpoint:    google.Endpoint,
		RedirectURL: "http://localhost:8080/oauth2callback",
	}

	var refreshToken string
	refreshToken, err = getRefreshToken()
	if err != nil {
		log.Printf("Failed to get refresh token: %v", err)
	}

	var token *oauth2.Token
	if refreshToken != "" {
		logus.Infof(ctx, "Found refresh token in keychain, exchanging for access token...")

		started := time.Now()

		token = &oauth2.Token{RefreshToken: refreshToken}
		ts := config.TokenSource(ctx, token) // Use a token source to get a fresh access token
		token, err = ts.Token()

		if err != nil {
			logus.Debugf(ctx, "Failed to refresh access token: %v", err)
		} else {
			logus.Debugf(ctx, "Exchanged refresh token for access token in %v", time.Since(started))
		}
	}

	if token == nil {
		//tok, err := tokenFromFile(tokFile)
		if token, err = getTokenFromWeb(ctx, config); err != nil {
			err = fmt.Errorf("failed to get token: %v", err)
			return
		}
		if token.RefreshToken != "" {
			if err = saveRefreshToken(token.RefreshToken); err != nil {
				log.Printf("Failed to save refresh token: %v", err)
			}
		}
	}
	return config.Client(ctx, token), nil
}

// saveRefreshToken securely stores a token in the system keychain
func saveRefreshToken(token string) error {
	log.Println("Saving refresh token to keyring...")
	return keyring.Set(keyringService, keyringUser, token)
}

// getRefreshToken retrieves a stored token from the keychain
func getRefreshToken() (string, error) {
	return keyring.Get(keyringService, keyringUser)
}
