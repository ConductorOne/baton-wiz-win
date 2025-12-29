//go:build !generate

package main

import (
	"context"

	"github.com/conductorone/baton-sdk/pkg/config"
	_ "github.com/conductorone/baton-sdk/pkg/connectorrunner"
	cfg "github.com/conductorone/baton-wiz-win/pkg/config"
	"github.com/conductorone/baton-wiz-win/pkg/connector"
)

var version = "dev"

func main() {
	ctx := context.Background()

	config.RunConnector(
		ctx,
		"baton-wiz-win",
		version,
		cfg.Config,
		connector.New,
		// connectorrunner.WithSessionStoreEnabled(), if the connector needs a cache.
	)
}
