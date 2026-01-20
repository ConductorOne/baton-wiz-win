package connector

import (
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
)

// userResourceType represents Wiz users.
var userResourceType = &v2.ResourceType{
	Id:          "user",
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
	Id:          "role",
	DisplayName: "Role",
	Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_ROLE},
	Annotations: annotations.New(
		&v2.CapabilityPermissions{
			Permissions: []*v2.CapabilityPermission{
				{Permission: "read:users"}, // Required for fetching user-to-role memberships
			},
		},
		&v2.SkipEntitlements{},
	),
}

// projectResourceType represents Wiz projects/workspaces.
var projectResourceType = &v2.ResourceType{
	Id:          "project",
	DisplayName: "Project",
	Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_GROUP},
	Annotations: annotations.New(
		&v2.CapabilityPermissions{
			Permissions: []*v2.CapabilityPermission{
				{Permission: "read:projects"},
			},
		},
		&v2.SkipEntitlements{},
	),
}

// securityInsightResourceType represents Wiz security insights/issues.
var securityInsightResourceType = &v2.ResourceType{
	Id:          "security-insight",
	DisplayName: "Security Insight",
	Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_SECURITY_INSIGHT},
	Annotations: annotations.New(
		&v2.CapabilityPermissions{
			Permissions: []*v2.CapabilityPermission{
				{Permission: "read:issues"},
			},
		},
		&v2.SkipEntitlementsAndGrants{},
	),
}
