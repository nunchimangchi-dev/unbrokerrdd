package email

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"google.golang.org/api/gmail/v1"
)

// WhoAmI reports which account the stored grant actually belongs to.
//
// This is not a convenience. The Gmail API sets From: to the authenticated
// account, and a deletion request is verified by brokers against the address
// they hold on file - so a grant on the wrong account sends every letter from
// an address the broker cannot match, while the body names a different one.
// The Cloud project here is owned by one account and the mail is meant to send
// as another, which is a supported arrangement and an easy one to get wrong at
// the sign-in prompt.
//
// gmail.compose cannot read the mailbox, so it cannot simply ask who the user
// is. It can create a draft, and Gmail stamps that draft with From: - so this
// creates one, reads the header back, and deletes it. Nothing is sent and
// nothing is left behind.
func WhoAmI(ctx context.Context, svc *gmail.Service) (string, error) {
	raw := "To: verify@invalid.example\r\nSubject: account check\r\n\r\ntemporary; deleted immediately\r\n"
	draft, err := svc.Users.Drafts.Create("me", &gmail.Draft{
		Message: &gmail.Message{Raw: base64.URLEncoding.EncodeToString([]byte(raw))},
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("create probe draft: %w", err)
	}
	// Always attempt cleanup, even if reading it back fails.
	defer func() {
		_ = svc.Users.Drafts.Delete("me", draft.Id).Context(ctx).Do()
	}()

	got, err := svc.Users.Drafts.Get("me", draft.Id).Format("metadata").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("read probe draft back: %w", err)
	}
	if got.Message == nil || got.Message.Payload == nil {
		return "", fmt.Errorf("draft came back without headers")
	}
	for _, h := range got.Message.Payload.Headers {
		if strings.EqualFold(h.Name, "From") {
			return h.Value, nil
		}
	}
	return "", fmt.Errorf("no From header on the draft")
}

// DraftSummary is one queued draft, for verification without leaving the
// terminal.
type DraftSummary struct {
	ID      string
	To      string
	Subject string
}

// ListDrafts returns only this tool's own deletion-request drafts.
//
// The filter is not cosmetic. The first version listed every draft in the
// account and printed a hundred of them - personal correspondence, recipients'
// names and addresses, subject lines - to answer a question about three
// letters. A tool granted compose access to someone's mailbox has no business
// showing their private drafts back to them, or to anyone reading its output,
// and "it was only metadata" is not a defence when the metadata is who someone
// writes to.
//
// Drafts are matched on the subject line this tool generates. Anything else in
// the mailbox is skipped without being read past its headers.
func ListDrafts(ctx context.Context, svc *gmail.Service) ([]DraftSummary, error) {
	res, err := svc.Users.Drafts.List("me").MaxResults(100).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list drafts: %w", err)
	}
	var out []DraftSummary
	for _, d := range res.Drafts {
		got, gErr := svc.Users.Drafts.Get("me", d.Id).Format("metadata").Context(ctx).Do()
		if gErr != nil || got.Message == nil || got.Message.Payload == nil {
			continue // unreadable, and not knowably ours - say nothing about it
		}
		s := DraftSummary{ID: d.Id}
		for _, h := range got.Message.Payload.Headers {
			switch strings.ToLower(h.Name) {
			case "to":
				s.To = h.Value
			case "subject":
				s.Subject = h.Value
			}
		}
		if !strings.HasPrefix(s.Subject, subjectPrefix) {
			continue // not ours; nothing about it is recorded or shown
		}
		out = append(out, s)
	}
	return out, nil
}
