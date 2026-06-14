package domain

// StatusOption is a selectable status value from the task-manager source.
type StatusOption struct {
	Name        string
	Description string
}

// TypeOption is a selectable issue type value from the task-manager source.
type TypeOption struct {
	Name        string
	Description string
}

// LabelOption is a selectable label value from the task-manager source.
type LabelOption struct {
	Name        string
	Description string
}
