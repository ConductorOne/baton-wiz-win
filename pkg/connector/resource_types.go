package connector

import (
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
)

// Resource type ID constants, referenced when gating cross-type grant
// emission against the sync filter (see cli.ConnectorOpts.WillSyncResourceType).
const (
	UserResourceTypeID    = "user"
	RoleResourceTypeID    = "role"
	ProjectResourceTypeID = "project"
)

// userResourceType represents Wiz users.
var userResourceType = &v2.ResourceType{
	Id:          UserResourceTypeID,
	DisplayName: "User",
	Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_USER},
	Annotations: annotations.New(
		&v2.CapabilityPermissions{
			Permissions: []*v2.CapabilityPermission{
				{Permission: "read:users"},
			},
		},
		&v2.SkipEntitlements{},
	),
}

// roleResourceType represents Wiz roles.
var roleResourceType = &v2.ResourceType{
	Id:          RoleResourceTypeID,
	DisplayName: "Role",
	Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_ROLE},
	Annotations: annotations.New(
		&v2.CapabilityPermissions{
			Permissions: []*v2.CapabilityPermission{
				{Permission: "read:users"},  // Required for fetching user-to-role memberships
				{Permission: "write:users"}, // Required to assign roles
			},
		},
		&v2.SkipEntitlements{},
	),
}

// projectResourceType represents Wiz projects/workspaces.
var projectResourceType = &v2.ResourceType{
	Id:          ProjectResourceTypeID,
	DisplayName: "Project",
	Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_GROUP},
	Annotations: annotations.New(
		&v2.CapabilityPermissions{
			Permissions: []*v2.CapabilityPermission{
				{Permission: "read:projects"},
				{Permission: "write:users"},    // Required to assign project members
				{Permission: "write:projects"}, // Required to assign project owners and security champions
			},
		},
		&v2.SkipEntitlements{},
	),
}
