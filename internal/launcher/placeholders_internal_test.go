package launcher

import "testing"

// TestIssueFieldPlaceholdersCoverEveryUntrustedPlaceholder is the parity guard
// for the shell-body check: every interpolation placeholder except the
// operator-trusted project root must be treated as untrusted issue content, so
// adding a new {{issue.*}} placeholder cannot silently exempt it.
func TestIssueFieldPlaceholdersCoverEveryUntrustedPlaceholder(t *testing.T) {
	t.Parallel()

	trusted := map[string]struct{}{projectRootPlaceholder: {}}

	untrusted := make(map[string]struct{}, len(issueFieldPlaceholders()))
	for _, placeholder := range issueFieldPlaceholders() {
		untrusted[placeholder] = struct{}{}
	}

	supported := (InterpolationContext{}).Placeholders()

	for placeholder := range supported {
		_, isTrusted := trusted[placeholder]
		_, isUntrusted := untrusted[placeholder]
		switch {
		case isTrusted && isUntrusted:
			t.Errorf("placeholder %s is classified both trusted and untrusted", placeholder)
		case !isTrusted && !isUntrusted:
			t.Errorf("placeholder %s is exempt from the shell-body guard; classify it as trusted or untrusted", placeholder)
		}
	}

	for placeholder := range untrusted {
		if _, ok := supported[placeholder]; !ok {
			t.Errorf("guarded placeholder %s is not a supported interpolation placeholder", placeholder)
		}
	}
}
