package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider"
)

// version is set by goreleaser via ldflags at release time.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/JeongJaeSoon/claude-enterprise",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
