// Package email sends CCPA/CPRA deletion requests through Gmail.
//
// Strategies 3 and 5 cover 37 of this project's targets and do not involve
// driving a broker's website at all - they send a statutory request to an
// address the broker publishes. That matters because every autonomous win the
// browser strategies failed to earn died at a bot defence, and a defence that
// blocks a headless browser has no bearing on an email. The blocker here is
// entirely on our side, which makes it the one remaining category that can be
// unblocked by writing code rather than by asking a broker's permission.
package email

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
)

// Scope is deliberately gmail.compose and nothing more.
//
// compose allows creating, updating and sending drafts. It does NOT grant
// read access to the mailbox. That is a real restriction with a real cost:
// this tool cannot read brokers' replies, so confirming an acknowledgement
// stays a human task for now. Reading the inbox would need gmail.readonly,
// which grants access to every message in the account - an entirely different
// thing to hand an automated agent, and a decision the account's owner should
// make deliberately rather than inherit from a convenience.
const Scope = gmail.GmailComposeScope

// TokenPath and CredentialsPath are resolved relative to the working directory
// so they sit beside .env, and both are gitignored.
const (
	CredentialsPath = "gmail_credentials.json"
	TokenPath       = "gmail_token.json"
)

// LoadConfig reads the OAuth client created in Google Cloud Console.
func LoadConfig() (*oauth2.Config, error) {
	b, err := os.ReadFile(CredentialsPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w (create an OAuth client in Google Cloud Console and save it here - see GMAIL.md)", CredentialsPath, err)
	}
	cfg, err := google.ConfigFromJSON(b, Scope)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", CredentialsPath, err)
	}
	return cfg, nil
}

// AuthURL returns the consent URL to visit, for the device-less flow.
//
// Uses AccessTypeOffline so Google issues a refresh token: without it the
// grant expires in an hour and every later run fails with an opaque 401 long
// after the person who authorised it has moved on. ApprovalForce makes Google
// re-issue the refresh token even when the account has consented before -
// Google only sends one on first consent, so a re-auth after deleting the
// token file would otherwise silently produce a credential that cannot renew.
func AuthURL(cfg *oauth2.Config) string {
	return cfg.AuthCodeURL("state-token",
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	)
}

// ExchangeAndSave turns the pasted authorisation code into a saved token.
func ExchangeAndSave(ctx context.Context, cfg *oauth2.Config, code string) error {
	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("exchange authorisation code: %w", err)
	}
	if tok.RefreshToken == "" {
		return fmt.Errorf("Google returned no refresh token; re-run the flow (the consent screen must be forced, not skipped)")
	}
	return saveToken(tok)
}

func saveToken(tok *oauth2.Token) error {
	b, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	// 0600: this file is a bearer credential for sending mail as the account
	// owner. It is as sensitive as a password and is never committed.
	if err := os.WriteFile(TokenPath, b, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", TokenPath, err)
	}
	abs, _ := filepath.Abs(TokenPath)
	fmt.Printf("saved refresh token to %s (mode 0600)\n", abs)
	return nil
}

// Client returns an authorised Gmail service, refreshing the access token as
// needed and persisting a rotated refresh token if Google issues one.
func Client(ctx context.Context) (*gmail.Service, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(TokenPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w (run `databrokergo auth-gmail` first)", TokenPath, err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, fmt.Errorf("parse %s: %w", TokenPath, err)
	}

	src := cfg.TokenSource(ctx, &tok)
	fresh, err := src.Token()
	if err != nil {
		return nil, fmt.Errorf("refresh access token: %w (the grant may have been revoked; re-run auth-gmail)", err)
	}
	// Google can rotate the refresh token. Dropping a rotated one leaves a
	// credential that works now and fails silently weeks later.
	if fresh.RefreshToken != "" && fresh.RefreshToken != tok.RefreshToken {
		if err := saveToken(fresh); err != nil {
			return nil, err
		}
	}

	svc, err := gmail.NewService(ctx, optionFromSource(src))
	if err != nil {
		return nil, fmt.Errorf("gmail service: %w", err)
	}
	return svc, nil
}
