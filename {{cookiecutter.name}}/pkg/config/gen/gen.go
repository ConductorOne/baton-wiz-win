package main

import (
	cfg "github.com/{{ cookiecutter.repo_owner }}/{{ cookiecutter.repo_name }}/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/config"
)

func main() {
	config.Generate("{{ cookiecutter.name_no_prefix }}", cfg.Config)
}
