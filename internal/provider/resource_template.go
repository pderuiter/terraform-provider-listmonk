package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

var (
	_ resource.Resource                   = &templateResource{}
	_ resource.ResourceWithImportState    = &templateResource{}
	_ resource.ResourceWithValidateConfig = &templateResource{}
)

const (
	templateTypeCampaign       = "campaign"
	templateTypeCampaignVisual = "campaign_visual"
	templateTypeTx             = "tx"
)

// NewTemplateResource returns the listmonk_template resource.
func NewTemplateResource() resource.Resource { return &templateResource{} }

type templateResource struct {
	client *client.Client
}

type templateModel struct {
	ID         types.Int64  `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Type       types.String `tfsdk:"type"`
	Subject    types.String `tfsdk:"subject"`
	Body       types.String `tfsdk:"body"`
	BodySource types.String `tfsdk:"body_source"`
	IsDefault  types.Bool   `tfsdk:"is_default"`
}

func (r *templateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_template"
}

func (r *templateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a listmonk e-mail template. Use `listmonk_default_template` to make a " +
			"campaign template the default.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "Numeric ID of the template.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the template.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 2000)},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "`campaign`, `campaign_visual` or `tx` (transactional). Changing it replaces the template.",
				Required:            true,
				Validators: []validator.String{stringvalidator.OneOf(
					templateTypeCampaign, templateTypeCampaignVisual, templateTypeTx)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"subject": schema.StringAttribute{
				MarkdownDescription: "Subject line. Required for `tx` templates and not allowed for campaign " +
					"templates (a campaign sets its own subject).",
				Optional: true,
			},
			"body": schema.StringAttribute{
				MarkdownDescription: "Template body (Go `html/template`). Campaign templates must contain " +
					"`{{ template \"content\" . }}`.",
				Required: true,
			},
			"body_source": schema.StringAttribute{
				MarkdownDescription: "Source document of the visual editor, for `campaign_visual` templates. " +
					"listmonk cannot clear it once set, so removing this argument keeps the current value.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the default campaign template.",
				Computed:            true,
			},
		},
	}
}

func (r *templateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg templateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Type.IsUnknown() || cfg.Type.IsNull() {
		return
	}

	if cfg.Type.ValueString() == templateTypeTx {
		if !cfg.Subject.IsUnknown() && (cfg.Subject.IsNull() || cfg.Subject.ValueString() == "") {
			resp.Diagnostics.AddAttributeError(path.Root("subject"), "Missing subject",
				"Transactional (tx) templates require a subject.")
		}
	} else if !cfg.Subject.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("subject"), "Subject not allowed",
			"Only tx templates have a subject; campaigns set their own. Remove the subject argument.")
	}
}

func (r *templateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromResource(req, resp)
}

func templatePayload(m templateModel) client.Template {
	t := client.Template{
		Name:    m.Name.ValueString(),
		Type:    m.Type.ValueString(),
		Subject: m.Subject.ValueString(),
		Body:    m.Body.ValueString(),
	}
	if !m.BodySource.IsNull() && !m.BodySource.IsUnknown() {
		s := m.BodySource.ValueString()
		t.BodySource = &s
	}
	return t
}

func (r *templateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan templateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.CreateTemplate(ctx, templatePayload(plan))
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "create", "template", err)
		return
	}

	// The create response omits the body on some versions; read it back.
	full, err := r.client.GetTemplate(ctx, out.ID)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "template", err)
		return
	}
	templateToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *templateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state templateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetTemplate(ctx, state.ID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "template", err)
		return
	}

	templateToModel(out, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *templateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state templateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueInt64()
	if err := r.client.UpdateTemplate(ctx, id, templatePayload(plan)); err != nil {
		apiErrorDiag(&resp.Diagnostics, "update", "template", err)
		return
	}

	out, err := r.client.GetTemplate(ctx, id)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "template", err)
		return
	}
	templateToModel(out, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *templateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state templateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteTemplate(ctx, state.ID.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete template",
			err.Error()+"\n\nlistmonk refuses to delete the default campaign template and the last "+
				"remaining template. Make another template the default first.")
	}
}

func (r *templateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importNumericID(ctx, req, resp)
}

func templateToModel(t *client.Template, m *templateModel) {
	m.ID = types.Int64Value(t.ID)
	m.Name = types.StringValue(t.Name)
	m.Type = types.StringValue(t.Type)
	m.Body = types.StringValue(t.Body)
	m.IsDefault = types.BoolValue(t.IsDefault)

	// Only tx templates have a meaningful subject. For campaign templates
	// listmonk's update query stores the template *name* as subject, which
	// must not show up as drift.
	if t.Type == templateTypeTx {
		m.Subject = types.StringValue(t.Subject)
	} else {
		m.Subject = types.StringNull()
	}

	if t.BodySource == nil || *t.BodySource == "" {
		m.BodySource = types.StringNull()
	} else {
		m.BodySource = types.StringValue(*t.BodySource)
	}
}
