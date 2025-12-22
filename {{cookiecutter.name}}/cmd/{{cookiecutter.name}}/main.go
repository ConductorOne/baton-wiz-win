//go:build !generate

package main

import (
	"context"

	"github.com/conductorone/baton-sdk/pkg/config"
	_ "github.com/conductorone/baton-sdk/pkg/connectorrunner"
	cfg "github.com/{{ cookiecutter.repo_owner }}/{{ cookiecutter.repo_name }}/pkg/config"
	"github.com/{{ cookiecutter.repo_owner }}/{{ cookiecutter.repo_name }}/pkg/connector"
)

var version = "dev"

func main() {
	ctx := context.Background()

	config.RunConnector(
		ctx,
		"{{ cookiecutter.name }}",
		version,
		cfg.Config,
		connector.New,
		// connectorrunner.WithSessionStoreEnabled(), if the connector needs a cache. 
	)
}
