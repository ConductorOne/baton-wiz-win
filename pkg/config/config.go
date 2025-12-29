package config

import (
	"github.com/conductorone/baton-sdk/pkg/field"
)

var (
	// Wiz authentication configuration fields
	wizAPIURL       = field.StringField("wiz-api-url", field.WithRequired(true), field.WithDescription("The Wiz GraphQL API endpoint (e.g., https://api.wiz.io/graphql)"))
	wizClientID     = field.StringField("wiz-client-id", field.WithRequired(true), field.WithDescription("OAuth2 client ID for Wiz API authentication"))
	wizClientSecret = field.StringField("wiz-client-secret", field.WithRequired(true), field.WithIsSecret(true), field.WithDescription("OAuth2 client secret for Wiz API authentication"))
	wizAuthEndpoint = field.StringField("wiz-auth-endpoint", field.WithRequired(true), field.WithDescription("OAuth2 token endpoint (e.g., https://auth.wiz.io/oauth/token)"))

	ConfigurationFields = []field.SchemaField{wizAPIURL, wizClientID, wizClientSecret, wizAuthEndpoint}

	// FieldRelationships defines relationships between the ConfigurationFields that can be automatically validated.
	FieldRelationships = []field.SchemaFieldRelationship{}
)

//go:generate go run -tags=generate ./gen
var Config = field.NewConfiguration(
	ConfigurationFields,
	field.WithConstraints(FieldRelationships...),
	field.WithConnectorDisplayName("Wiz"),
)
