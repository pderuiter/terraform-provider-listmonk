package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

var (
	_ resource.Resource                = &settingResource{}
	_ resource.ResourceWithImportState = &settingResource{}
)

// settingsMu serialises settings writes: every write makes listmonk reload
// itself, and overlapping reloads only make the retries longer.
var settingsMu sync.Mutex

// NewSettingResource returns the listmonk_setting resource.
func NewSettingResource() resource.Resource { return &settingResource{} }

type settingResource struct {
	client *client.Client
}

type settingModel struct {
	ID    types.String         `tfsdk:"id"`
	Key   types.String         `tfsdk:"key"`
	Value jsontypes.Normalized `tfsdk:"value"`
}

func (r *settingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_setting"
}

func (r *settingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one key of the listmonk settings (Settings in the admin UI), for example " +
			"`app.site_name`, `app.root_url`, `privacy.individual_tracking` or `smtp`.\n\n" +
			"listmonk reloads itself after a settings change; the provider waits for it to come back.\n\n" +
			"Settings always exist, so destroying this resource only stops managing the key: the value stays " +
			"as it is.\n\n" +
			"Secrets (SMTP passwords, API keys) are masked when listmonk returns settings, so changes to them " +
			"made outside Terraform are not detected. Fields that listmonk adds on its own (such as the `uuid` " +
			"of each SMTP server) are not drift either: only the fields in `value` are compared.\n\n" +
			"For `smtp`, every server must set the duration fields the listmonk version has (`idle_timeout`, " +
			"`wait_timeout` and, from v6, `msg_retry_delay`). A field left out is stored as `\"\"` by the next " +
			"full settings save and listmonk then fails to start, so the provider rejects such a value.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Same as `key`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "Settings key, as returned by `GET /api/settings`, e.g. `app.site_name`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "Value as JSON, e.g. `jsonencode(\"My newsletter\")`, `jsonencode(true)` or " +
					"`jsonencode([{ host = \"smtp.example.com\", ... }])`.",
				CustomType: jsontypes.NormalizedType{},
				Required:   true,
			},
		},
	}
}

func (r *settingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromResource(req, resp)
}

// write sets the key and confirms that the server holds the value.
func (r *settingResource) write(ctx context.Context, key string, value jsontypes.Normalized) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	settings, err := r.client.GetSettings(ctx)
	if err != nil {
		return err
	}
	if _, ok := settings[key]; !ok {
		// listmonk would silently ignore an unknown key.
		return fmt.Errorf("listmonk has no setting %q; check the key against GET /api/settings", key)
	}

	raw := json.RawMessage(value.ValueString())
	if key == "smtp" {
		if err := client.CheckSMTPDurations(raw, settings[key]); err != nil {
			return err
		}
	}
	if err := r.client.UpdateSetting(ctx, key, raw); err != nil {
		return err
	}

	settings, err = r.client.GetSettings(ctx)
	if err != nil {
		return err
	}
	if !client.SettingMatches(raw, settings[key]) {
		return fmt.Errorf("listmonk accepted setting %q but now returns %s. The value may be invalid for this key, "+
			"contain fields this listmonk version does not know (they are dropped), or be normalised by listmonk "+
			"(e.g. lower-cased domains, a root URL without trailing slash); set it the way listmonk stores it",
			key, string(settings[key]))
	}
	return nil
}

func (r *settingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan settingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.write(ctx, plan.Key.ValueString(), plan.Value); err != nil {
		apiErrorDiag(&resp.Diagnostics, "set", "setting "+plan.Key.ValueString(), err)
		return
	}
	plan.ID = plan.Key
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *settingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state settingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	settings, err := r.client.GetSettings(ctx)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "settings", err)
		return
	}
	server, ok := settings[state.Key.ValueString()]
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}

	// Keep the configured value while the server still satisfies it (masked
	// secrets, server-added fields); otherwise surface the real value as drift.
	if state.Value.IsNull() || !client.SettingMatches(json.RawMessage(state.Value.ValueString()), server) {
		state.Value = jsontypes.NewNormalizedValue(string(server))
	}
	state.ID = state.Key
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *settingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan settingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.write(ctx, plan.Key.ValueString(), plan.Value); err != nil {
		apiErrorDiag(&resp.Diagnostics, "set", "setting "+plan.Key.ValueString(), err)
		return
	}
	plan.ID = plan.Key
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete only forgets the key: listmonk settings cannot be removed.
func (r *settingResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning("Setting left in place",
		"listmonk settings cannot be deleted. The key is no longer managed by Terraform and keeps its current value.")
}

func (r *settingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("value"), jsontypes.NewNormalizedNull())...)
}
