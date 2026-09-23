package agent

import (
	"strings"
	"testing"
)

func TestBuildSearchURL_Placeholders(t *testing.T) {
	got, err := BuildSearchURL("https://x.com/s?q={first+last}&full={name}&l={last}", "Ada King Lovelace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"q=Ada%2BLovelace", "full=Ada+King+Lovelace", "l=Lovelace"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in %q", want, got)
		}
	}
}

func TestBuildSearchURL_RejectsUnsubstituted(t *testing.T) {
	// A leftover placeholder would be sent to the site literally, producing a
	// page that searched for nothing - which reads as "no results", which
	// would classify as absent. Fail loudly instead.
	if _, err := BuildSearchURL("https://x.com/s?q={firstname}", "Ada Lovelace"); err == nil {
		t.Fatal("expected an error for an unsubstituted placeholder")
	}
}

func TestBuildSearchURL_RequiresTwoNameParts(t *testing.T) {
	if _, err := BuildSearchURL("https://x.com/s?q={first+last}", "Cher"); err == nil {
		t.Fatal("expected an error for a single-word name")
	}
}

func TestBuildSearchURL_ErrorDoesNotLeakName(t *testing.T) {
	_, err := BuildSearchURL("https://x.com/s?q={first+last}", "Cher")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "Cher") {
		t.Errorf("error leaked the subject name: %q", err)
	}
}

func TestScrubSubject(t *testing.T) {
	// The exact shape that leaked in practice: a classifier quoting the name
	// back inside its evidence, on its way to the database.
	in := "No results for Warren Shubin; warren shubin was not found."
	got := ScrubSubject(in, "Warren Shubin")
	for _, bad := range []string{"Warren", "Shubin", "warren", "shubin"} {
		if strings.Contains(got, bad) {
			t.Errorf("ScrubSubject left %q in %q", bad, got)
		}
	}
	if !strings.Contains(got, "W***") || !strings.Contains(got, "S***") {
		t.Errorf("expected masked initials, got %q", got)
	}
}

func TestRedactURL_MasksQueryAndPathNames(t *testing.T) {
	got := RedactURL("https://x.com/name/ada-lovelace?q=Ada+Lovelace&page=2")
	for _, bad := range []string{"Ada", "Lovelace", "ada", "lovelace"} {
		if strings.Contains(got, bad) {
			t.Errorf("RedactURL leaked %q in %q", bad, got)
		}
	}
}

func TestSearchTemplates_AreAbsoluteHTTPS(t *testing.T) {
	// A relative or http template would either fail to navigate or downgrade
	// the connection while carrying the subject's name in the query string.
	for id, tmpl := range SearchTemplates {
		if !strings.HasPrefix(tmpl, "https://") {
			t.Errorf("%s: template must be absolute https, got %q", id, tmpl)
		}
		if !strings.Contains(tmpl, "{") {
			t.Errorf("%s: template has no name placeholder, so it searches for nothing: %q", id, tmpl)
		}
	}
}
