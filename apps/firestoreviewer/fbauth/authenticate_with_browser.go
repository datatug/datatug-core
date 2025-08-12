package fbauth

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/oauth2"
	"log"
	"net/http"
	"os/exec"
	"runtime"
)

// getTokenFromWeb runs a browser auth flow.
func getTokenFromWeb(ctx context.Context, config *oauth2.Config) (token *oauth2.Token, err error) {
	// Step 1: Get auth URL
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	// Step 2: Open browser
	if err = openBrowser(authURL); err != nil {
		return
	}

	// Step 3: Wait for redirect with code
	var authCode string
	if authCode, err = waitForAuthCode(); err != nil {
		log.Printf("Failed to get auth code: %v", err)
	}

	// Step 4: Exchange code for token
	token, err = config.Exchange(ctx, authCode)
	if err != nil {
		log.Fatalf("Token exchange error: %v", err)
	}
	return
}

func openBrowser(url string) (err error) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
	case "windows":
		cmd = "rundll32"
		args = append(args, "url.dll,FileProtocolHandler")
	case "darwin":
		cmd = "open"
	default:
		fmt.Printf("Please open this URL manually: %s\n", url)
		return
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// Starts HTTP server to capture OAuth redirect
func waitForAuthCode() (authCode string, err error) {
	ch := make(chan string)
	srv := &http.Server{Addr: ":8080"}

	http.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		_, _ = fmt.Fprintln(w, "Login successful! You can close this window.")
		go func() {
			ch <- code
			if err := srv.Shutdown(context.Background()); err != nil {
				log.Print("Failed to shutdown:", err)
			}
		}()
	})

	go func() {
		if err = srv.ListenAndServe(); errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return <-ch, err
}
