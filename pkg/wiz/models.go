package wiz

// PageInfo represents GraphQL pagination information using Relay cursor pagination.
type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// UserRoleRef represents a reference to a user's role.
type UserRoleRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProjectRef represents a reference to a project assigned to a user.
type ProjectRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// User represents a Wiz user (from users query).
type User struct {
	ID                        string       `json:"id"`
	Email                     string       `json:"email"`
	Name                      string       `json:"name"`
	EffectiveRole             UserRoleRef  `json:"effectiveRole"`
	EffectiveAssignedProjects []ProjectRef `json:"effectiveAssignedProjects"`
}

// UserConnection represents a paginated list of users.
type UserConnection struct {
	Nodes    []User   `json:"nodes"`
	PageInfo PageInfo `json:"pageInfo"`
}

// UserRole represents a Wiz role/permission level.
type UserRole struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Scopes          []string `json:"scopes"`
	Builtin         bool     `json:"builtin"`
	IsProjectScoped bool     `json:"isProjectScoped"`
}

// UserRoleConnection represents a paginated list of user roles.
type UserRoleConnection struct {
	Nodes    []UserRole `json:"nodes"`
	PageInfo PageInfo   `json:"pageInfo"`
}

// ProjectOwner represents an owner of a project.
type ProjectOwner struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// SecurityChampion represents a security champion for a project.
type SecurityChampion struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// Project represents a Wiz project/workspace.
type Project struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Description       string             `json:"description"`
	ProjectOwners     []ProjectOwner     `json:"projectOwners"`
	SecurityChampions []SecurityChampion `json:"securityChampions"`
}

// ProjectConnection represents a paginated list of projects.
type ProjectConnection struct {
	Nodes    []Project `json:"nodes"`
	PageInfo PageInfo  `json:"pageInfo"`
}

// GraphQL response wrapper types.
type graphQLResponse struct {
	Data   interface{}    `json:"data"`
	Errors []graphQLError `json:"errors,omitempty"`
}

type graphQLError struct {
	Message string        `json:"message"`
	Path    []interface{} `json:"path,omitempty"`
}

// Specific response types for each query.
type projectsQueryResponse struct {
	Projects ProjectConnection `json:"projects"`
}
