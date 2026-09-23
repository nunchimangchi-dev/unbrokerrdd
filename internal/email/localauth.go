package email

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// AuthorizeLocal runs the loopback redirect flow that modern Desktop OAuth
// clients require.
//
// Google retired the out-of-band flow (urn:ietf:wg:oauth:2.0:oob) in 2022, so
// a Desktop client's redirect_uris is "http://localhost" and there is no code
// displayed for a person to copy - the code arrives as a query parameter on a
// redirect to a port nothing is listening on. Without a listener the browser
// shows a connection error and the code is only recoverable by reading it out
// of the URL bar, which works and is a good way to paste the wrong string.
//
// RFC 8252 lets a loopback redirect use any port, so this binds an ephemeral
// one and tells Google about it at request time.
func AuthorizeLocal(ctx context.Context, cfg *oauth2.Config, timeout time.Duration) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start local callback listener: %w", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	local := *cfg
	local.RedirectURL = fmt.Sprintf("http://localhost:%d", port)

	type result struct {
		code string
		err  error
	}
	results := make(chan result, 1)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if e := q.Get("error"); e != "" {
				http.Error(w, "authorisation denied: "+e, http.StatusBadRequest)
				results <- result{err: fmt.Errorf("authorisation denied: %s", e)}
				return
			}
			code := q.Get("code")
			if code == "" {
				return // favicon and other stray requests
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<!doctype html><meta charset="utf-8">
<title>Authorised</title>
<body style="font:16px system-ui;padding:3rem;max-width:34rem">
<h1 style="font-size:1.25rem">Authorised</h1>
<p>unbrokerrdd can now create and send Gmail drafts as you.</p>
<p style="color:#555">This grant cannot read your mailbox. You can close this tab
and return to the terminal.</p>`)
			results <- result{code: code}
		}),
	}
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())

	fmt.Println("Open this URL and approve:")
	fmt.Printf("\n%s\n\n", AuthURLFor(&local))
	fmt.Printf("Waiting for the redirect on localhost:%d (up to %s)...\n", port, timeout)

	select {
	case res := <-results:
		if res.err != nil {
			return res.err
		}
		// Exchange against the same config the code was issued to: Google
		// checks redirect_uri again at exchange time, and a mismatch fails
		// with an unhelpful invalid_grant.
		return ExchangeAndSave(ctx, &local, res.code)
	case <-time.After(timeout):
		return fmt.Errorf("timed out after %s waiting for the browser redirect", timeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}
