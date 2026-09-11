package domain

import "sort"

// SortByLastChange orders issues in place newest change first: UpdatedAt
// descending, then Priority ascending, then ID ascending. Every browse
// surface that lists active work uses it, and the board's age markers rely
// on the order to draw one divider per threshold. A stable sort keeps equal
// items in their original relative order.
func SortByLastChange(issues []IssueSummary) {
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.ID < b.ID
	})
}
