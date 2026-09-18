package strategies

import "testing"

func TestSplitName(t *testing.T) {
	cases := []struct{ in, first, last string }{
		{"", "", ""},
		{"Madonna", "Madonna", ""},
		{"First Last", "First", "Last"},
		{"First Middle Last", "First", "Middle Last"},
	}
	for _, c := range cases {
		first, last := splitName(c.in)
		if first != c.first || last != c.last {
			t.Errorf("splitName(%q) = (%q, %q), want (%q, %q)", c.in, first, last, c.first, c.last)
		}
	}
}
