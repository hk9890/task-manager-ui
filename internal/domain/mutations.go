package domain

// CreateIssueInput defines supported create fields.
type CreateIssueInput struct {
	Title       string
	Description string
	Type        string
	Priority    *int
	Assignee    string
	Labels      []string
}

// CreateIssueResult returns key fields from a create operation.
type CreateIssueResult struct {
	IssueID string
}

// UpdateIssueInput defines partial update fields.
// Nil pointer fields are omitted from the update.
type UpdateIssueInput struct {
	Title       *string
	Description *string
	Status      *string
	Type        *string
	Priority    *int
	Assignee    *string
	Labels      []string
	ClearLabels bool
}

// CloseIssueInput defines close-operation inputs.
type CloseIssueInput struct {
	Reason string
}

// AddCommentInput defines supported comment input.
type AddCommentInput struct {
	Body string
}
