// Package provider implements the listmonk Terraform provider.
package provider

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

// Environment variables read when the matching provider argument is unset.
const (
	envURL      = "LISTMONK_URL"
	envUsername = "LISTMONK_USERNAME"
	envToken    = "LISTMONK_API_TOKEN" // #nosec G101 -- variable name, not a credential
	envInsecure = "LISTMONK_INSECURE_SKIP_VERIFY"
	envTimeout  = "LISTMONK_TIMEOUT"
)

var _ provider.Provider = &listmonkProvider{}

type listmonkProvider struct {
	version string
}

type providerModel struct {
	URL                types.String `tfsdk:"url"`
	Username           types.String `tfsdk:"username"`
	APIToken           types.String `tfsdk:"api_token"`
	InsecureSkipVerify types.Bool   `tfsdk:"insecure_skip_verify"`
	Timeout            types.Int64  `tfsdk:"timeout"`
}

// New returns a provider factory for the given version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &listmonkProvider{version: version}
	}
}

func (p *listmonkProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "listmonk"
	resp.Version = p.version
}

func (p *listmonkProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a [listmonk](https://listmonk.app) newsletter and mailing list manager " +
			"through its REST API. Authenticate with an API user (Admin → Users → type **API**).",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				MarkdownDescription: "Root URL of the listmonk instance, e.g. `https://listmonk.example.com`. " +
					"Can also be set with the `" + envURL + "` environment variable.",
				Optional: true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Username of the listmonk API user. " +
					"Can also be set with the `" + envUsername + "` environment variable.",
				Optional: true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Token of the listmonk API user. " +
					"Can also be set with the `" + envToken + "` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"insecure_skip_verify": schema.BoolAttribute{
				MarkdownDescription: "Skip TLS certificate verification. Defaults to `false`. " +
					"Can also be set with the `" + envInsecure + "` environment variable.",
				Optional: true,
			},
			"timeout": schema.Int64Attribute{
				MarkdownDescription: "Timeout in seconds for each API request. Defaults to `30`. " +
					"Can also be set with the `" + envTimeout + "` environment variable.",
				Optional: true,
			},
		},
	}
}

func (p *listmonkProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for attr, v := range map[string]types.String{"url": cfg.URL, "username": cfg.Username, "api_token": cfg.APIToken} {
		if v.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(attr), "Unknown listmonk provider value",
				"The provider cannot be configured with a value that is only known after apply. "+
					"Set it statically or use an environment variable.")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	url := stringOrEnv(cfg.URL, envURL)
	username := stringOrEnv(cfg.Username, envUsername)
	token := stringOrEnv(cfg.APIToken, envToken)

	insecure := false
	if !cfg.InsecureSkipVerify.IsNull() && !cfg.InsecureSkipVerify.IsUnknown() {
		insecure = cfg.InsecureSkipVerify.ValueBool()
	} else if v := os.Getenv(envInsecure); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			resp.Diagnostics.AddError("Invalid "+envInsecure, err.Error())
			return
		}
		insecure = b
	}

	timeout := 30 * time.Second
	if !cfg.Timeout.IsNull() && !cfg.Timeout.IsUnknown() {
		timeout = time.Duration(cfg.Timeout.ValueInt64()) * time.Second
	} else if v := os.Getenv(envTimeout); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			resp.Diagnostics.AddError("Invalid "+envTimeout, err.Error())
			return
		}
		timeout = time.Duration(n) * time.Second
	}

	if url == "" {
		resp.Diagnostics.AddAttributeError(path.Root("url"), "Missing listmonk URL",
			"Set the url provider argument or the "+envURL+" environment variable.")
	}
	if username == "" {
		resp.Diagnostics.AddAttributeError(path.Root("username"), "Missing listmonk API username",
			"Set the username provider argument or the "+envUsername+" environment variable.")
	}
	if token == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_token"), "Missing listmonk API token",
			"Set the api_token provider argument or the "+envToken+" environment variable.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c, err := client.New(client.Config{
		BaseURL:            url,
		Username:           username,
		Token:              token,
		Timeout:            timeout,
		InsecureSkipVerify: insecure,
		UserAgent:          "terraform-provider-listmonk/" + p.version,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create listmonk client", err.Error())
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *listmonkProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewListResource,
		NewTemplateResource,
		NewDefaultTemplateResource,
		NewSubscriberResource,
		NewUserRoleResource,
		NewListRoleResource,
		NewUserResource,
		NewSettingResource,
	}
}

func (p *listmonkProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewListDataSource,
		NewListsDataSource,
		NewTemplateDataSource,
		NewUserRoleDataSource,
	}
}

func stringOrEnv(v types.String, env string) string {
	if !v.IsNull() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(env)
}
