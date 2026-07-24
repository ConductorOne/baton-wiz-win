package connector

import (
	"context"
	"fmt"
	"io"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/cli"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	cfg "github.com/conductorone/baton-wiz-win/pkg/config"
	"github.com/conductorone/baton-wiz-win/pkg/wiz"
)

type Connector struct {
	client wiz.Client

	// syncRoles and syncProjects reflect whether the "role" and "project"
	// resource types are included in the current sync filter. When either is
	// false, userBuilder skips emitting the corresponding cross-type grant
	// (see userBuilder.Grants).
	syncRoles    bool
	syncProjects bool
}

// ResourceSyncers returns a ResourceSyncer for each resource type that should be synced from the upstream service.
func (c *Connector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncerV2 {
	return []connectorbuilder.ResourceSyncerV2{
		newUserBuilder(c.client, c.syncRoles, c.syncProjects),
		newRoleBuilder(c.client),
		newProjectBuilder(c.client),
	}
}

// Asset takes an input AssetRef and attempts to fetch it using the connector's authenticated http client
// It streams a response, always starting with a metadata object, following by chunked payloads for the asset.
func (c *Connector) Asset(ctx context.Context, asset *v2.AssetRef) (string, io.ReadCloser, error) {
	return "", nil, nil
}

// Metadata returns metadata about the connector.
func (c *Connector) Metadata(ctx context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "Wiz",
		Description: "Wiz cloud security platform connector for syncing users, roles, and projects",
	}, nil
}

// Validate is called to ensure that the connector is properly configured. It should exercise any API credentials
// to be sure that they are valid.
func (c *Connector) Validate(ctx context.Context) (annotations.Annotations, error) {
	// Test the API credentials by attempting to list user roles
	_, err := c.client.ListUserRoles(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to validate Wiz API credentials: %w", err)
	}
	return nil, nil
}

// New returns a new instance of the connector.
func New(ctx context.Context,
	connectorConfig *cfg.WizWin,
	cliOpts *cli.ConnectorOpts,
) (connectorbuilder.ConnectorBuilderV2,
	[]connectorbuilder.Opt,
	error,
) {
	// Use base-url override if provided, otherwise use wiz-api-url
	apiURL := connectorConfig.WizApiUrl
	if connectorConfig.BaseUrl != "" {
		apiURL = connectorConfig.BaseUrl
	}

	// Initialize the Wiz API client
	client, err := wiz.NewClient(
		ctx,
		apiURL,
		connectorConfig.WizClientId,
		connectorConfig.WizClientSecret,
		connectorConfig.WizAuthEndpoint,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Wiz client: %w", err)
	}

	// Default to syncing everything when cliOpts is unavailable, matching
	// WillSyncResourceType's own "no filter set" semantics.
	syncRoles := true
	syncProjects := true
	if cliOpts != nil {
		syncRoles = cliOpts.WillSyncResourceType(RoleResourceTypeID)
		syncProjects = cliOpts.WillSyncResourceType(ProjectResourceTypeID)
	}

	return &Connector{
		client:       client,
		syncRoles:    syncRoles,
		syncProjects: syncProjects,
	}, nil, nil
}
