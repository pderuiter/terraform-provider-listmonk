package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

var (
	_ resource.Resource                = &listResource{}
	_ resource.ResourceWithImportState = &listResource{}
)

// NewListResource returns the listmonk_list resource.
func NewListResource() resource.Resource { return &listResource{} }

type listResource struct {
	client *client.Client
}

type listModel struct {
	ID          types.Int64  `tfsdk:"id"`
	UUID        types.String `tfsdk:"uuid"`
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Optin       types.String `tfsdk:"optin"`
	Status      types.String `tfsdk:"status"`
	Tags        types.Set    `tfsdk:"tags"`
	Description types.String `tfsdk:"description"`
}

// Tags are stored by listmonk with whitespace runs replaced by "-", so only
// tags without whitespace round-trip unchanged.
var tagRegexp = regexp.MustCompile(`^\S+$`)

func (r *listResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_list"
}

func (r *listResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a listmonk mailing list.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "Numeric ID of the list.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "UUID of the list.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the list.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 2000)},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "`public` lists are shown on the public subscription page; `private` lists are not. Defaults to `private`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("private"),
				Validators:          []validator.String{stringvalidator.OneOf("public", "private")},
			},
			"optin": schema.StringAttribute{
				MarkdownDescription: "`single` subscribes immediately; `double` sends a confirmation e-mail first. Defaults to `single`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("single"),
				Validators:          []validator.String{stringvalidator.OneOf("single", "double")},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "`active` or `archived`. Defaults to `active`. Archiving requires listmonk v6 or later.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("active"),
				Validators:          []validator.String{stringvalidator.OneOf("active", "archived")},
			},
			"tags": schema.SetAttribute{
				MarkdownDescription: "Tags on the list. Tags cannot contain whitespace (listmonk would rewrite it to `-`).",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Default:             setdefault.StaticValue(stringSetValue(nil)),
				Validators: []validator.Set{setvalidator.ValueStringsAre(
					stringvalidator.RegexMatches(tagRegexp, "must not contain whitespace"),
					stringvalidator.LengthBetween(1, 100),
				)},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the list. listmonk cannot clear a description once set, " +
					"so removing this argument keeps the current description.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *listResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromResource(req, resp)
}

func (r *listResource) payload(ctx context.Context, m listModel, d *diag.Diagnostics) client.List {
	return client.List{
		Name:        m.Name.ValueString(),
		Type:        m.Type.ValueString(),
		Optin:       m.Optin.ValueString(),
		Status:      m.Status.ValueString(),
		Tags:        setToStrings(ctx, m.Tags, d),
		Description: m.Description.ValueString(),
	}
}

func (r *listResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan listModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.payload(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.CreateList(ctx, payload)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "create", "list", err)
		return
	}

	plan.ID = types.Int64Value(out.ID)
	checkListStatusSupport(out, plan.Status.ValueString(), &resp.Diagnostics)
	listToModel(out, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *listResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state listModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetList(ctx, state.ID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "list", err)
		return
	}

	listToModel(out, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *listResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state listModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.payload(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.UpdateList(ctx, state.ID.ValueInt64(), payload)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "update", "list", err)
		return
	}

	plan.ID = state.ID
	checkListStatusSupport(out, plan.Status.ValueString(), &resp.Diagnostics)
	listToModel(out, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *listResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state listModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteList(ctx, state.ID.ValueInt64()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "delete", "list", err)
	}
}

func (r *listResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importNumericID(ctx, req, resp)
}

// checkListStatusSupport turns a silently ignored status (listmonk < v6 has
// no list status) into a clear error instead of an "inconsistent result".
func checkListStatusSupport(out *client.List, planned string, d *diag.Diagnostics) {
	if out.Status == "" && planned != "" && planned != "active" {
		d.AddError("List status not supported",
			"This listmonk server does not support list status (added in listmonk v6). "+
				"Remove status = \""+planned+"\" or upgrade listmonk.")
	}
}

func listToModel(l *client.List, m *listModel) {
	m.ID = types.Int64Value(l.ID)
	m.UUID = types.StringValue(l.UUID)
	m.Name = types.StringValue(l.Name)
	m.Type = types.StringValue(l.Type)
	m.Optin = types.StringValue(l.Optin)
	// listmonk < v6 has no list status; every list is effectively active.
	if l.Status == "" {
		m.Status = types.StringValue("active")
	} else {
		m.Status = types.StringValue(l.Status)
	}
	m.Tags = stringSetValue(l.Tags)
	if l.Description == "" {
		m.Description = types.StringNull()
	} else {
		m.Description = types.StringValue(l.Description)
	}
}
