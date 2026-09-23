package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/chromedp"
)

// NewBrowserContext returns a chromedp context, reusing an existing browser
// when the parent context already has one.
//
// This reuse is the point. Each chromedp browser gets its own temporary
// user-data-dir (~65MB), removed on clean shutdown - and a browser killed by a
// context deadline never shuts down cleanly. Creating one browser per site
// meant a sweep over 85 domains leaked 85 profiles, and on this machine those
// land inside the flatpak sandbox on /run/user/1000, a 1.6GB tmpfs. One
// afternoon of sweeps filled it to 100%, at which point bwrap could not write
// its info_fd and NO sandboxed application could start - not the automation,
// not anything else the user runs under flatpak. The tooling took out part of
// the desktop.
//
// When the parent already holds a browser, chromedp.NewContext opens a tab in
// it instead of launching another, so a sweep costs one profile rather than
// one per target. Callers doing more than a single page should create one root
// context and pass it down.
func NewBrowserContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if chromedp.FromContext(ctx) != nil {
		return chromedp.NewContext(ctx) // new tab in the existing browser
	}

	path := os.Getenv("CHROME_PATH")
	if path == "" {
		return chromedp.NewContext(ctx)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(path))
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	return taskCtx, func() {
		cancelTask()
		cancelAlloc()
	}
}

// NewSweepBrowser starts one browser for a whole sweep. Callers must call the
// returned cancel, which shuts the browser down and lets chromedp remove its
// profile directory.
func NewSweepBrowser(ctx context.Context) (context.Context, context.CancelFunc, error) {
	browserCtx, cancel := NewBrowserContext(ctx)

	// Warm the browser up on browserCtx itself, never on a derived context.
	// chromedp binds the browser it starts to whichever context first runs an
	// action; warming up on a context.WithTimeout child meant the deferred
	// cancel of that child tore the browser down, and every subsequent target
	// failed with "context canceled" - a sweep that reported five brokers
	// unreachable when the browser had been killed by its own warm-up.
	//
	// The preflight in CheckBrowser has already established that a browser can
	// start, so this does not need its own deadline.
	if err := chromedp.Run(browserCtx, chromedp.Navigate("about:blank")); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("start browser: %w", err)
	}
	return browserCtx, cancel, nil
}

// SweepProfileDirs reports leftover chromedp profile directories and their
// total size, so a sweep can say plainly when it is leaking.
func SweepProfileDirs() (count int, bytes int64) {
	for _, base := range profileSearchPaths() {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "chromedp-runner") {
				continue
			}
			count++
			bytes += dirSize(filepath.Join(base, e.Name()))
		}
	}
	return count, bytes
}

func profileSearchPaths() []string {
	paths := []string{os.TempDir()}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		// Where the flatpak sandbox's /tmp actually lands on the host.
		paths = append(paths, filepath.Join(xdg, ".flatpak", "com.google.Chrome", "tmp"))
	}
	return paths
}

func dirSize(path string) int64 {
	var total int64
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil && !d.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}
