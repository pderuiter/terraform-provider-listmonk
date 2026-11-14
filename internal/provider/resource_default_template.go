package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

var (
	_ resource.Resource                = &defaultTemplateResource{}
	_ resource.ResourceWithImportState = &defaultTemplateResource{}
)

// NewDefaultTemplateResource returns the listmonk_default_template resource.
func NewDefaultTemplateResource() resource.Resource { return &defaultTemplateResource{} }

type defaultTemplateResource struct {
	client *client.Client
}

type defaultTemplateModel struct {
	ID         types.String `tfsdk:"id"`
	TemplateID types.Int64  `tfsdk:"template_id"`
}

// defaultTemplateID is the fixed ID of this singleton resource.
const defaultTemplateID = "default"

func (r *defaultTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_default_template"
}

func (r *defaultTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sets the default campaign template of the listmonk instance. There can be only one " +
			"default, so declare this resource at most once. Destroying it leaves the current default in place: " +
			"listmonk always has a default template.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `default`.",
				Computed:            true,
			},
			"template_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the campaign template to make the default.",
				Required:            true,
			},
		},
	}
}

func (r *defaultTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromResource(req, resp)
}

func (r *defaultTemplateResource) set(ctx context.Context, m *defaultTemplateModel) error {
	id := m.TemplateID.ValueInt64()
	if err := r.client.SetDefaultTemplate(ctx, id); err != nil {
		return err
	}

	// listmonk silently ignores anything that is not a "campaign" template
	// (its query filters on type), so confirm the change took.
	templates, err := r.client.GetTemplates(ctx)
	if err != nil {
		return err
	}
	for _, t := range templates {
		if t.ID == id {
			if !t.IsDefault {
				return fmt.Errorf("template %d (type %q) was not made the default; only templates of type %q can be the default",
					id, t.Type, templateTypeCampaign)
			}
			m.ID = types.StringValue(defaultTemplateID)
			return nil
		}
	}
	return fmt.Errorf("template %d does not exist", id)
}

func (r *defaultTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan defaultTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.set(ctx, &plan); err != nil {
		apiErrorDiag(&resp.Diagnostics, "set", "default template", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *defaultTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state defaultTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	templates, err := r.client.GetTemplates(ctx)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "templates", err)
		return
	}
	for _, t := range templates {
		if t.IsDefault && (t.Type == templateTypeCampaign || t.Type == templateTypeCampaignVisual) {
			state.TemplateID = types.Int64Value(t.ID)
			break
		}
	}
	state.ID = types.StringValue(defaultTemplateID)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *defaultTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan defaultTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.set(ctx, &plan); err != nil {
		apiErrorDiag(&resp.Diagnostics, "set", "default template", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete only forgets the resource: listmonk always needs a default template.
func (r *defaultTemplateResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *defaultTemplateResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.Set(ctx, defaultTemplateModel{
		ID:         types.StringValue(defaultTemplateID),
		TemplateID: types.Int64Value(0),
	})...)
}
