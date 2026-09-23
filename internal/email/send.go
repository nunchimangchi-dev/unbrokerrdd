package email

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

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

// SendDraft sends a draft that already exists in the account, by id.
//
// This is how --send must work whenever a draft exists. Recomposing the
// message from the template instead would silently discard whatever the person
// changed while reading it in Gmail - which is the entire purpose of drafting
// first. A tool that says "review these, then send" and then sends something
// other than what was reviewed is worse than one that never offered the
// review.
func SendDraft(ctx context.Context, svc *gmail.Service, draftID string) (string, error) {
	msg, err := svc.Users.Drafts.Send("me", &gmail.Draft{Id: draftID}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("send draft %s: %w", draftID, err)
	}
	return msg.Id, nil
}

// DuplicateRecipients returns any address with more than one queued draft.
//
// Sending both would deliver the same statutory request twice, from a real
// person, to a company that keeps records - so this is checked before sending
// rather than reported after.
func DuplicateRecipients(drafts []DraftSummary) map[string]int {
	seen := map[string]int{}
	for _, d := range drafts {
		seen[strings.ToLower(strings.TrimSpace(d.To))]++
	}
	dupes := map[string]int{}
	for to, n := range seen {
		if n > 1 {
			dupes[to] = n
		}
	}
	return dupes
}
