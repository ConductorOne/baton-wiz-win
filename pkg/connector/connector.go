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
}

// ResourceSyncers returns a ResourceSyncer for each resource type that should be synced from the upstream service.
func (c *Connector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncerV2 {
	return []connectorbuilder.ResourceSyncerV2{
		newUserBuilder(c.client),
		newRoleBuilder(c.client),
		newProjectBuilder(c.client),
		newInsightBuilder(c.client),
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
		Description: "Wiz cloud security platform connector for syncing users, roles, projects, and security insights",
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
	// Initialize the Wiz API client
	client, err := wiz.NewClient(
		ctx,
		connectorConfig.WizApiUrl,
		connectorConfig.WizClientId,
		connectorConfig.WizClientSecret,
		connectorConfig.WizAuthEndpoint,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Wiz client: %w", err)
	}

	return &Connector{client: client}, nil, nil
}
