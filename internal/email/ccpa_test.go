package email

import (
	"strings"
	"testing"
)

func validRequest() Request {
	return Request{
		BrokerName:   "Example Broker",
		BrokerEmail:  "privacy@example.com",
		SubjectName:  "Ada Lovelace",
		SubjectMail:  "ada@example.org",
		SubjectState: "OH",
	}
}

func TestRequest_Validate_RequiresEverything(t *testing.T) {
	// An incomplete statutory request is worse than none: it starts the clock,
	// gets rejected as unverifiable, and leaves a record suggesting the matter
	// was handled.
	for _, tc := range []struct {
		name  string
		mutate func(*Request)
		want  string
	}{
		{"no broker email", func(r *Request) { r.BrokerEmail = "" }, "broker email"},
		{"no subject name", func(r *Request) { r.SubjectName = "" }, "SUBJECT_NAME"},
		{"no subject email", func(r *Request) { r.SubjectMail = "" }, "SUBJECT_EMAIL"},
		{"no state", func(r *Request) { r.SubjectState = "" }, "SUBJECT_STATE"},
		{"whitespace only", func(r *Request) { r.SubjectName = "   " }, "SUBJECT_NAME"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validRequest()
			tc.mutate(&r)
			err := r.Validate()
			if err == nil {
				t.Fatal("expected validation to fail")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should name the missing field %q", err, tc.want)
			}
		})
	}
}

func TestRequest_Validate_AcceptsComplete(t *testing.T) {
	if err := validRequest().Validate(); err != nil {
		t.Fatalf("a complete request should validate: %v", err)
	}
}

func TestRequest_BodyCarriesTheIdentifyingFacts(t *testing.T) {
	body, err := validRequest().Body()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"Ada Lovelace", "ada@example.org", "OH", "Example Broker"} {
		if !strings.Contains(body, want) {
			t.Errorf("body is missing %q — a broker cannot locate the record without it", want)
		}
	}
	if strings.Contains(body, "{{") {
		t.Error("body has an unrendered template placeholder")
	}
}

func TestRequest_RFC822_HeadersAreWellFormed(t *testing.T) {
	raw, err := validRequest().RFC822()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(raw, "To: privacy@example.com\r\n") {
		t.Errorf("To: header missing or malformed, got %.60q", raw)
	}
	if !strings.Contains(raw, "\r\nSubject: Consumer Request to Delete Personal Information - Ada Lovelace\r\n") {
		t.Error("Subject header missing or malformed")
	}
	// A bare \n in an RFC822 message truncates or corrupts it at some servers.
	headerEnd := strings.Index(raw, "\r\n\r\n")
	if headerEnd < 0 {
		t.Fatal("no header/body separator")
	}
	for i := 0; i < headerEnd; i++ {
		if raw[i] == '\n' && (i == 0 || raw[i-1] != '\r') {
			t.Fatalf("bare LF in headers at offset %d", i)
		}
	}
}

func TestScope_IsComposeNotReadAll(t *testing.T) {
	// gmail.readonly would grant access to every message in the account. That
	// is a different thing to hand an automated agent, and must be a
	// deliberate decision rather than something that drifts in.
	if !strings.Contains(Scope, "gmail.compose") {
		t.Errorf("scope = %q, want gmail.compose", Scope)
	}
	if strings.Contains(Scope, "readonly") || strings.Contains(Scope, "mail.google.com") {
		t.Errorf("scope %q grants mailbox read access; this tool must not have it", Scope)
	}
}

func TestRequest_NeverClaimsCaliforniaResidencyForOtherStates(t *testing.T) {
	// The bug this guards: the first version of the letter opened with "I am a
	// California-resident-equivalent consumer" for every subject, including an
	// Ohio resident. Asserting a residency you do not have, in a request whose
	// force comes from being a verifiable statutory claim, is a misstatement
	// made in the sender's name.
	for _, state := range []string{"OH", "NY", "WY", "PR", ""} {
		r := validRequest()
		r.SubjectState = state
		body, err := r.Body()
		if err != nil {
			t.Fatalf("render %q: %v", state, err)
		}
		low := strings.ToLower(body)
		if strings.Contains(low, "california resident") || strings.Contains(low, "california-resident") {
			t.Errorf("state %q: letter claims California residency:\n%s", state, body)
		}
		if strings.Contains(low, "consumer privacy act") {
			t.Errorf("state %q: letter cites a statute that does not apply", state)
		}
		if !strings.Contains(low, "published privacy policy") {
			t.Errorf("state %q: neutral letter should fall back to the broker's own policy", state)
		}
	}
}

func TestRequest_CitesTheStatuteForCoveredStates(t *testing.T) {
	r := validRequest()
	r.SubjectState = "CA"
	body, err := r.Body()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(body, "California resident") {
		t.Error("a California resident's letter should say so")
	}
	if !strings.Contains(body, "1798.105") {
		t.Error("a California letter should cite the deletion provision")
	}
}

func TestRequest_StateMatchingIsCaseAndSpaceInsensitive(t *testing.T) {
	// SUBJECT_STATE comes from a hand-edited .env; " ca " must not silently
	// downgrade a Californian to the neutral letter.
	r := validRequest()
	r.SubjectState = " ca "
	if !strings.Contains(r.Preamble(), "California resident") {
		t.Errorf("preamble for %q did not match CA", r.SubjectState)
	}
}

func TestSubjectPrefix_IsStable(t *testing.T) {
	// The prefix is what keeps a draft listing out of the account owner's
	// private mail. A listing that matched nothing would silently widen to
	// everything if the filter were ever relaxed, so the coupling between the
	// generated subject and the filter is asserted rather than assumed.
	r := validRequest()
	if !strings.HasPrefix(r.Subject(), subjectPrefix) {
		t.Fatalf("Subject() = %q does not start with the filter prefix %q", r.Subject(), subjectPrefix)
	}
	if subjectPrefix == "" {
		t.Fatal("empty prefix would match every draft in the mailbox")
	}
}

func TestDuplicateRecipients(t *testing.T) {
	drafts := []DraftSummary{
		{ID: "a", To: "privacy@one.example"},
		{ID: "b", To: "Privacy@One.Example "}, // same address, different case and spacing
		{ID: "c", To: "privacy@two.example"},
	}
	dupes := DuplicateRecipients(drafts)
	if len(dupes) != 1 {
		t.Fatalf("got %d duplicate recipients, want 1: %v", len(dupes), dupes)
	}
	if dupes["privacy@one.example"] != 2 {
		t.Errorf("case and whitespace must not hide a duplicate: %v", dupes)
	}
}

func TestDuplicateRecipients_NoneWhenDistinct(t *testing.T) {
	drafts := []DraftSummary{
		{ID: "a", To: "privacy@one.example"},
		{ID: "b", To: "privacy@two.example"},
	}
	if d := DuplicateRecipients(drafts); len(d) != 0 {
		t.Errorf("distinct recipients reported as duplicates: %v", d)
	}
}
