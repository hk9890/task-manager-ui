package styles

// NormalizeIssueToken exposes the unexported normalizeIssueToken function for
// parity testing from package styles_test (which can import renderhelpers
// without creating a cycle that the production code cannot).
var NormalizeIssueToken = normalizeIssueToken
