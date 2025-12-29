package main

import (
	"github.com/conductorone/baton-sdk/pkg/config"
	cfg "github.com/conductorone/baton-wiz-win/pkg/config"
)

func main() {
	config.Generate("wiz-win", cfg.Config)
}
