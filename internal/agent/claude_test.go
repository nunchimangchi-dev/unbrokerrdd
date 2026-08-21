package agent

import "testing"

func TestCheckDomTextFailure_MatchesKnownSignals(t *testing.T) {
	cases := []struct {
		name    string
		domText string
	}{
		{"invalid email lowercase", "Sorry, that's an invalid email address."},
		{"invalid email mixed case", "Error: Invalid Email provided."},
		{"captcha", "Please complete the CAPTCHA to continue."},
		{"recaptcha", "This site is protected by reCAPTCHA."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, message := checkDomTextFailure(tc.domText)
			if !matched {
				t.Errorf("expected a failure match for %q, got none", tc.domText)
			}
			if message == "" {
				t.Error("expected a non-empty message on match")
			}
		})
	}
}

func TestCheckDomTextFailure_NeverMatchesOnSuccessLikeOrCleanText(t *testing.T) {
	// This is the important direction: potential-success and ambiguous text
	// must NEVER trigger a match here, since this function only short-circuits
	// failures - successes always still need real vision validation.
	cases := []struct {
		name    string
		domText string
	}{
		{"empty", ""},
		{"clean privacy page, no error", "Data Removal Center - Manage your privacy preferences here."},
		{"explicit success text", "Your request has been submitted. Check your email for confirmation."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, _ := checkDomTextFailure(tc.domText)
			if matched {
				t.Errorf("expected no failure match for %q, but got one", tc.domText)
			}
		})
	}
}
