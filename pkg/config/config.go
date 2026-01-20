package config

import (
	"github.com/conductorone/baton-sdk/pkg/field"
)

var (
	// Wiz authentication configuration fields
	wizAPIURL = field.StringField(
		"wiz-api-url",
		field.WithRequired(true),
		field.WithDisplayName("Wiz API URL"),
		field.WithDescription("The Wiz GraphQL API endpoint for your region"),
		field.WithPlaceholder("https://api.us17.app.wiz.io/graphql"),
	)
	wizClientID = field.StringField(
		"wiz-client-id",
		field.WithRequired(true),
		field.WithDisplayName("Client ID"),
		field.WithDescription("OAuth2 client ID from your Wiz service account"),
	)
	wizClientSecret = field.StringField(
		"wiz-client-secret",
		field.WithRequired(true),
		field.WithIsSecret(true),
		field.WithDisplayName("Client Secret"),
		field.WithDescription("OAuth2 client secret from your Wiz service account"),
	)
	wizAuthEndpoint = field.StringField(
		"wiz-auth-endpoint",
		field.WithRequired(true),
		field.WithDisplayName("Auth Endpoint"),
		field.WithDescription("OAuth2 token endpoint for authentication"),
		field.WithPlaceholder("https://auth.app.wiz.io/oauth/token"),
	)

	ConfigurationFields = []field.SchemaField{wizAPIURL, wizClientID, wizClientSecret, wizAuthEndpoint}

	// FieldRelationships defines relationships between the ConfigurationFields that can be automatically validated.
	FieldRelationships = []field.SchemaFieldRelationship{}
)

//go:generate go run -tags=generate ./gen
var Config = field.NewConfiguration(
	ConfigurationFields,
	field.WithConstraints(FieldRelationships...),
	field.WithConnectorDisplayName("Wiz"),
	field.WithIconUrl("/static/app-icons/wiz.svg"),
	field.WithHelpUrl("/docs/baton/wiz"),
)
