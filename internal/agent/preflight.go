package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// BrowserCheck reports whether headless Chrome actually works right now.
type BrowserCheck struct {
	ExecPath string
	Flatpak  bool
	OK       bool
	Err      string
}

// CheckBrowser starts a headless browser and loads a data: URL - no network,
// no site involved - purely to establish that the tool works before any sweep
// draws conclusions from it.
//
// This exists because a sweep with a broken browser does not fail, it
// produces. A discovery run over 22 brokers returned "contact found 0" when
// Chrome could not start at all; every one of those rows looked like a finding
// about a broker and was a finding about this machine. The project's recurring
// mistake is concluding from evidence that was never about the site, and a
// dead browser is the purest form of it - so the browser is now checked before
// the questions are asked, and a sweep refuses to start rather than filling a
// database with noise.
func CheckBrowser(ctx context.Context) *BrowserCheck {
	c := &BrowserCheck{ExecPath: findChrome()}

	if c.ExecPath != "" {
		if b, err := os.ReadFile(c.ExecPath); err == nil && strings.Contains(string(b), "flatpak run") {
			// `flatpak run` hands off to an already-running instance instead
			// of starting a new one with the flags chromedp passed, so the
			// remote-debugging port never opens and chromedp waits for a
			// websocket that will never exist.
			c.Flatpak = true
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	taskCtx, cancelTask := NewBrowserContext(checkCtx)
	defer cancelTask()

	var title string
	err := chromedp.Run(taskCtx,
		chromedp.Navigate("data:text/html,<title>preflight</title>ok"),
		chromedp.Title(&title),
	)
	if err != nil {
		c.Err = err.Error()
		return c
	}
	c.OK = title == "preflight"
	if !c.OK {
		c.Err = fmt.Sprintf("browser started but returned an unexpected title %q", title)
	}
	return c
}

func findChrome() string {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// Explain renders an actionable diagnosis, or "" when the browser is fine.
func (c *BrowserCheck) Explain() string {
	if c.OK {
		return ""
	}
	var b strings.Builder
	b.WriteString("headless browser is not usable, so nothing can be concluded about any site.\n")
	if c.ExecPath == "" {
		b.WriteString("  no chrome/chromium binary found on PATH\n")
	} else {
		fmt.Fprintf(&b, "  binary: %s\n", c.ExecPath)
	}
	if c.Flatpak {
		b.WriteString("  this is a flatpak wrapper (`flatpak run com.google.Chrome`).\n")
		b.WriteString("  flatpak attaches to a running instance instead of starting one with\n")
		b.WriteString("  the requested flags, so the remote-debugging port never opens and\n")
		b.WriteString("  chromedp waits for a websocket that never appears.\n")
		b.WriteString("  fix: install a native chromium for automation and point at it with\n")
		b.WriteString("       CHROME_PATH=/usr/bin/chromium\n")
	}
	if c.Err != "" {
		fmt.Fprintf(&b, "  error: %.200s\n", c.Err)
	}
	return b.String()
}
