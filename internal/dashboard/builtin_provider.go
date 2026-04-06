package dashboard

import (
	"context"
	"os"
	"strings"

	"github.com/hk9890/beads-workbench/internal/domain"
)

const (
	builtInDashboardIDDefault     = "default"
	builtInDashboardTitleDefault  = "Default"
	builtInSectionIDNotReady      = "not_ready"
	builtInSectionTitleNotReady   = "Not Ready"
	builtInSectionIDReady         = "ready"
	builtInSectionTitleReady      = "Ready"
	builtInSectionIDInProgress    = "in_progress"
	builtInSectionTitleInProgress = "In Progress"
	builtInSectionIDDone          = "done"
	builtInSectionTitleDone       = "Done"
	beadsActorEnvVar              = "BEADS_ACTOR"
	defaultSectionLimit           = 25
	inProgressStatus              = "in_progress"
	doneStatus                    = "closed"
)

// BuiltInProvider is a dashboard definition provider backed by built-in queue
// definitions mapped to supported gateway query contracts.
type BuiltInProvider struct {
	actorResolver ActorResolver
}

var _ DashboardDefinitionProvider = (*BuiltInProvider)(nil)

// ActorResolver resolves the current beads actor/user identity.
type ActorResolver interface {
	CurrentActor() string
}

// EnvActorResolver resolves current user identity from environment.
type EnvActorResolver struct {
	envKey string
}

// NewBuiltInProvider creates a built-in dashboard provider.
func NewBuiltInProvider() *BuiltInProvider {
	return &BuiltInProvider{actorResolver: NewEnvActorResolver()}
}

// NewBuiltInProviderWithActorResolver creates a built-in provider using a
// custom actor resolver. Empty resolver falls back to environment resolution.
func NewBuiltInProviderWithActorResolver(actorResolver ActorResolver) *BuiltInProvider {
	if actorResolver == nil {
		actorResolver = NewEnvActorResolver()
	}

	return &BuiltInProvider{actorResolver: actorResolver}
}

// NewEnvActorResolver creates an environment-based actor resolver.
func NewEnvActorResolver() EnvActorResolver {
	return EnvActorResolver{envKey: beadsActorEnvVar}
}

// CurrentActor returns the current actor identity from BEADS_ACTOR.
func (r EnvActorResolver) CurrentActor() string {
	if r.envKey == "" {
		return ""
	}

	return strings.TrimSpace(os.Getenv(r.envKey))
}

// Dashboards returns built-in dashboard definitions.
func (p *BuiltInProvider) Dashboards(_ context.Context) ([]Definition, error) {
	sections := []Section{
		notReadySection(),
		readySection(),
		inProgressSection(),
		doneSection(),
	}

	return []Definition{{
		ID:       builtInDashboardIDDefault,
		Title:    builtInDashboardTitleDefault,
		Sections: sections,
	}}, nil
}

func notReadySection() Section {
	return Section{
		ID:    builtInSectionIDNotReady,
		Title: builtInSectionTitleNotReady,
		Query: Query{
			Type:          QueryTypeBlockedIssues,
			BlockedIssues: domain.BlockedIssuesQuery{Limit: defaultSectionLimit},
		},
	}
}

func readySection() Section {
	return Section{
		ID:    builtInSectionIDReady,
		Title: builtInSectionTitleReady,
		Query: Query{
			Type:        QueryTypeReadyIssues,
			ReadyIssues: domain.ReadyIssuesQuery{Limit: defaultSectionLimit},
		},
	}
}

func inProgressSection() Section {
	return Section{
		ID:    builtInSectionIDInProgress,
		Title: builtInSectionTitleInProgress,
		Query: Query{
			Type: QueryTypeListIssues,
			ListIssues: domain.IssueListQuery{
				Statuses: []string{inProgressStatus},
				Limit:    defaultSectionLimit,
			},
		},
	}
}

func doneSection() Section {
	return Section{
		ID:    builtInSectionIDDone,
		Title: builtInSectionTitleDone,
		Query: Query{
			Type: QueryTypeListIssues,
			ListIssues: domain.IssueListQuery{
				Statuses:  []string{doneStatus},
				SortBy:    domain.SortFieldUpdatedAt,
				SortOrder: domain.SortDirectionDescending,
				Limit:     defaultSectionLimit,
			},
		},
	}
}
