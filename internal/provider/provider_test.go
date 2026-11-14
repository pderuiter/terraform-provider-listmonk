package provider

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	fwschema "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

// testAccProtoV6ProviderFactories runs the provider in-process for
// acceptance tests (TF_ACC=1). Credentials come from LISTMONK_URL,
// LISTMONK_USERNAME and LISTMONK_API_TOKEN; scripts/testacc-local.sh sets
// them up against a throwaway listmonk.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"listmonk": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	for _, env := range []string{envURL, envUsername, envToken} {
		if os.Getenv(env) == "" {
			t.Fatalf("%s must be set for acceptance tests (see scripts/testacc-local.sh)", env)
		}
	}
}

// testAccClient returns an API client for CheckDestroy functions.
func testAccClient(t *testing.T) *client.Client {
	t.Helper()
	c, err := client.New(client.Config{
		BaseURL:  os.Getenv(envURL),
		Username: os.Getenv(envUsername),
		Token:    os.Getenv(envToken),
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestProviderSchemas validates every resource and data source schema
// without a listmonk server.
func TestProviderSchemas(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	for _, f := range p.Resources(ctx) {
		r := f()
		var resp fwschema.SchemaResponse
		r.Schema(ctx, fwschema.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%T: %v", r, resp.Diagnostics)
		}
		if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Errorf("%T: %v", r, diags)
		}
		if _, ok := r.(resource.ResourceWithImportState); !ok {
			t.Errorf("%T does not support import", r)
		}
	}
}
