package taskmgr

import (
	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

func cloneStrings(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	return out
}

func toSummary(i *tasks.Issue) domain.IssueSummary {
	return domain.IssueSummary{
		ID:        i.ID,
		Title:     i.Title,
		Status:    string(i.Status),
		Type:      string(i.Type),
		Priority:  i.Priority,
		Assignee:  i.Assignee,
		Labels:    cloneStrings(i.Labels),
		CreatedAt: i.Created,
		UpdatedAt: i.Updated,
	}
}

func toSummaries(items []*tasks.Issue) []domain.IssueSummary {
	out := make([]domain.IssueSummary, 0, len(items))
	for _, i := range items {
		out = append(out, toSummary(i))
	}
	return out
}

func toRef(r tasks.Ref) domain.IssueReference {
	return domain.IssueReference{
		ID:       r.ID,
		Title:    r.Title,
		Type:     string(r.Type),
		Priority: r.Priority,
		Status:   string(r.Status),
	}
}

func toRefs(rs []tasks.Ref) []domain.IssueReference {
	out := make([]domain.IssueReference, 0, len(rs))
	for _, r := range rs {
		out = append(out, toRef(r))
	}
	return out
}

func toComment(c tasks.Comment) domain.IssueComment {
	return domain.IssueComment{
		ID:        c.ID,
		Author:    c.Author,
		Body:      c.Body,
		CreatedAt: c.Created,
	}
}

func toComments(cs []tasks.Comment) []domain.IssueComment {
	out := make([]domain.IssueComment, 0, len(cs))
	for _, c := range cs {
		out = append(out, toComment(c))
	}
	return out
}

func toBlockedViews(items []tasks.BlockedIssue) []domain.BlockedIssueView {
	out := make([]domain.BlockedIssueView, 0, len(items))
	for _, bi := range items {
		out = append(out, domain.BlockedIssueView{
			Issue:     toSummary(bi.Issue),
			BlockedBy: toRefs(bi.BlockedBy),
		})
	}
	return out
}

// toDetail projects a resolved SDK Detail onto the taskmgr-ui detail read model. Notes
// has no SDK counterpart (task-manager stores a single markdown body), so it is
// left empty. ParentGroupBrowser.Parent is set from the resolved parent ref;
// IssueDetail.Children comes from the SDK's derived children.
func toDetail(d *tasks.Detail) domain.IssueDetail {
	det := domain.IssueDetail{
		Summary:     toSummary(&d.Issue),
		Creator:     d.Creator,
		Description: d.Description,
		Notes:       "",
		ClosedAt:    d.Closed,
		CloseReason: d.CloseReason,
		BlockedBy:   toRefs(d.BlockedByRefs),
		Blocks:      toRefs(d.Blocks),
		Related:     toRefs(d.RelatedRefs),
		Children:    toRefs(d.Children),
		Comments:    toComments(d.Comments),
	}
	if d.ParentRef != nil {
		det.ParentGroupBrowser.Parent = toRef(*d.ParentRef)
	}
	return det
}

// staticCatalogs builds the selectable option sets. Status and type values are
// fixed enums in task-manager, so those catalogs are constant; labels reflect
// the distinct labels currently in use.
func staticCatalogs(labels []string) repository.Catalogs {
	statuses := make([]domain.StatusOption, 0, len(tasks.Statuses))
	for _, s := range tasks.Statuses {
		statuses = append(statuses, domain.StatusOption{Name: string(s), Description: statusDescription(s)})
	}
	types := make([]domain.TypeOption, 0, len(tasks.Types))
	for _, t := range tasks.Types {
		types = append(types, domain.TypeOption{Name: string(t), Description: typeDescription(t)})
	}
	labelOpts := make([]domain.LabelOption, 0, len(labels))
	for _, l := range labels {
		labelOpts = append(labelOpts, domain.LabelOption{Name: l})
	}
	return repository.Catalogs{Statuses: statuses, Types: types, Labels: labelOpts}
}

func statusDescription(s tasks.Status) string {
	switch s {
	case tasks.StatusOpen:
		return "Available to work"
	case tasks.StatusInProgress:
		return "Actively being worked on"
	case tasks.StatusBlocked:
		return "Blocked by a dependency"
	case tasks.StatusDeferred:
		return "Consciously postponed"
	case tasks.StatusClosed:
		return "Completed or closed"
	default:
		return ""
	}
}

func typeDescription(t tasks.Type) string {
	switch t {
	case tasks.TypeTask:
		return "A unit of work"
	case tasks.TypeBug:
		return "A defect to fix"
	case tasks.TypeFeature:
		return "New functionality"
	case tasks.TypeEpic:
		return "A grouping of related issues"
	case tasks.TypeChore:
		return "Maintenance or housekeeping"
	default:
		return ""
	}
}
