package email

import (
	"context"
	"encoding/base64"
	"fmt"

	"google.golang.org/api/gmail/v1"
)

// CreateDraft puts the request in the account's Drafts folder and returns the
// draft id. Nothing leaves the account.
//
// Drafting is the default path for a reason. These messages go out in a real
// person's name, make statements on his behalf, and cannot be recalled. A
// draft keeps the last step - the irreversible one - with the human whose name
// is on it, and costs one click.
func CreateDraft(ctx context.Context, svc *gmail.Service, r Request) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	raw, err := r.RFC822()
	if err != nil {
		return "", err
	}
	draft, err := svc.Users.Drafts.Create("me", &gmail.Draft{
		Message: &gmail.Message{Raw: base64.URLEncoding.EncodeToString([]byte(raw))},
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("create draft for %s: %w", r.BrokerName, err)
	}
	return draft.Id, nil
}

// Send delivers the request immediately. Callers must have explicit
// per-invocation authorisation from the account owner; nothing in this package
// calls it on its own initiative.
func Send(ctx context.Context, svc *gmail.Service, r Request) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	raw, err := r.RFC822()
	if err != nil {
		return "", err
	}
	msg, err := svc.Users.Messages.Send("me", &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString([]byte(raw)),
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("send to %s: %w", r.BrokerName, err)
	}
	return msg.Id, nil
}
