package email

import (
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

// option wraps a TokenSource for the Gmail client, isolated here so the
// google.golang.org/api/option import does not shadow local variable names
// called "option" elsewhere in the package.
func optionFromSource(src oauth2.TokenSource) option.ClientOption {
	return option.WithTokenSource(src)
}
