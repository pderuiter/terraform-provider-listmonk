package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

// ---------------------------------------------------------------------------
// listmonk_list
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource                     = &listDataSource{}
	_ datasource.DataSourceWithConfigValidators = &listDataSource{}
)

// NewListDataSource returns the listmonk_list data source.
func NewListDataSource() datasource.DataSource { return &listDataSource{} }

type listDataSource struct {
	client *client.Client
}

type listDataModel struct {
	ID              types.Int64  `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	UUID            types.String `tfsdk:"uuid"`
	Type            types.String `tfsdk:"type"`
	Optin           types.String `tfsdk:"optin"`
	Status          types.String `tfsdk:"status"`
	Tags            types.Set    `tfsdk:"tags"`
	Description     types.String `tfsdk:"description"`
	SubscriberCount types.Int64  `tfsdk:"subscriber_count"`
}

var listDataAttrTypes = map[string]attr.Type{
	"id":               types.Int64Type,
	"name":             types.StringType,
	"uuid":             types.StringType,
	"type":             types.StringType,
	"optin":            types.StringType,
	"status":           types.StringType,
	"tags":             types.SetType{ElemType: types.StringType},
	"description":      types.StringType,
	"subscriber_count": types.Int64Type,
}

func listDataAttributes(lookup bool) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			MarkdownDescription: "Numeric ID of the list.",
			Optional:            lookup,
			Computed:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "Name of the list.",
			Optional:            lookup,
			Computed:            true,
		},
		"uuid":        schema.StringAttribute{MarkdownDescription: "UUID of the list.", Computed: true},
		"type":        schema.StringAttribute{MarkdownDescription: "`public` or `private`.", Computed: true},
		"optin":       schema.StringAttribute{MarkdownDescription: "`single` or `double`.", Computed: true},
		"status":      schema.StringAttribute{MarkdownDescription: "`active` or `archived`.", Computed: true},
		"tags":        schema.SetAttribute{MarkdownDescription: "Tags on the list.", ElementType: types.StringType, Computed: true},
		"description": schema.StringAttribute{MarkdownDescription: "Description of the list.", Computed: true},
		"subscriber_count": schema.Int64Attribute{
			MarkdownDescription: "Number of subscribers (only filled in by `listmonk_list`).",
			Computed:            true,
		},
	}
}

func (d *listDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_list"
}

func (d *listDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up one listmonk list by `id` or by exact `name`.",
		Attributes:          listDataAttributes(true),
	}
}

func (d *listDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *listDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromDataSource(req, resp)
}

func (d *listDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg listDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := cfg.ID.ValueInt64()
	if cfg.ID.IsNull() {
		lists, err := d.client.GetLists(ctx)
		if err != nil {
			apiErrorDiag(&resp.Diagnostics, "read", "lists", err)
			return
		}
		var matches []int64
		for _, l := range lists {
			if l.Name == cfg.Name.ValueString() {
				matches = append(matches, l.ID)
			}
		}
		switch len(matches) {
		case 0:
			resp.Diagnostics.AddError("List not found", fmt.Sprintf("No list named %q.", cfg.Name.ValueString()))
			return
		case 1:
			id = matches[0]
		default:
			resp.Diagnostics.AddError("Ambiguous list name",
				fmt.Sprintf("%d lists are named %q (IDs %v); look the list up by id instead.", len(matches), cfg.Name.ValueString(), matches))
			return
		}
	}

	l, err := d.client.GetList(ctx, id)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "list", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, listToDataModel(l))...)
}

func listToDataModel(l *client.List) listDataModel {
	status := l.Status
	if status == "" {
		status = "active"
	}
	return listDataModel{
		ID:              types.Int64Value(l.ID),
		Name:            types.StringValue(l.Name),
		UUID:            types.StringValue(l.UUID),
		Type:            types.StringValue(l.Type),
		Optin:           types.StringValue(l.Optin),
		Status:          types.StringValue(status),
		Tags:            stringSetValue(l.Tags),
		Description:     types.StringValue(l.Description),
		SubscriberCount: types.Int64Value(l.SubscriberCount),
	}
}

// ---------------------------------------------------------------------------
// listmonk_lists
// ---------------------------------------------------------------------------

var _ datasource.DataSource = &listsDataSource{}

// NewListsDataSource returns the listmonk_lists data source.
func NewListsDataSource() datasource.DataSource { return &listsDataSource{} }

type listsDataSource struct {
	client *client.Client
}

type listsDataModel struct {
	Tag   types.String `tfsdk:"tag"`
	Type  types.String `tfsdk:"type"`
	Lists types.List   `tfsdk:"lists"`
}

func (d *listsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lists"
}

func (d *listsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns all listmonk lists the API user can see, optionally filtered.",
		Attributes: map[string]schema.Attribute{
			"tag": schema.StringAttribute{
				MarkdownDescription: "Only return lists with this tag.",
				Optional:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Only return lists of this type (`public` or `private`).",
				Optional:            true,
			},
			"lists": schema.ListNestedAttribute{
				MarkdownDescription: "The lists, ordered by ID. `subscriber_count` is always 0 here; use `listmonk_list` for counts.",
				Computed:            true,
				NestedObject:        schema.NestedAttributeObject{Attributes: listDataAttributes(false)},
			},
		},
	}
}

func (d *listsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromDataSource(req, resp)
}

func (d *listsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg listsDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	lists, err := d.client.GetLists(ctx)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "lists", err)
		return
	}
	slices.SortFunc(lists, func(a, b client.List) int { return int(a.ID - b.ID) })

	objType := types.ObjectType{AttrTypes: listDataAttrTypes}
	elems := []attr.Value{}
	for i := range lists {
		l := &lists[i]
		if !cfg.Tag.IsNull() && !slices.Contains(l.Tags, cfg.Tag.ValueString()) {
			continue
		}
		if !cfg.Type.IsNull() && l.Type != cfg.Type.ValueString() {
			continue
		}
		obj, diags := types.ObjectValueFrom(ctx, listDataAttrTypes, listToDataModel(l))
		resp.Diagnostics.Append(diags...)
		elems = append(elems, obj)
	}
	cfg.Lists = types.ListValueMust(objType, elems)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// ---------------------------------------------------------------------------
// listmonk_template
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource                     = &templateDataSource{}
	_ datasource.DataSourceWithConfigValidators = &templateDataSource{}
)

// NewTemplateDataSource returns the listmonk_template data source.
func NewTemplateDataSource() datasource.DataSource { return &templateDataSource{} }

type templateDataSource struct {
	client *client.Client
}

type templateDataModel struct {
	ID        types.Int64  `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	Subject   types.String `tfsdk:"subject"`
	Body      types.String `tfsdk:"body"`
	IsDefault types.Bool   `tfsdk:"is_default"`
}

func (d *templateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_template"
}

func (d *templateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up one listmonk template by `id` or by exact `name`, e.g. to use a template " +
			"created in the UI or the built-in default.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.Int64Attribute{MarkdownDescription: "Numeric ID of the template.", Optional: true, Computed: true},
			"name":       schema.StringAttribute{MarkdownDescription: "Name of the template.", Optional: true, Computed: true},
			"type":       schema.StringAttribute{MarkdownDescription: "`campaign`, `campaign_visual` or `tx`.", Computed: true},
			"subject":    schema.StringAttribute{MarkdownDescription: "Subject of a `tx` template.", Computed: true},
			"body":       schema.StringAttribute{MarkdownDescription: "Template body.", Computed: true},
			"is_default": schema.BoolAttribute{MarkdownDescription: "Whether this is the default campaign template.", Computed: true},
		},
	}
}

func (d *templateDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *templateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromDataSource(req, resp)
}

func (d *templateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg templateDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := cfg.ID.ValueInt64()
	if cfg.ID.IsNull() {
		templates, err := d.client.GetTemplates(ctx)
		if err != nil {
			apiErrorDiag(&resp.Diagnostics, "read", "templates", err)
			return
		}
		var matches []int64
		for _, t := range templates {
			if t.Name == cfg.Name.ValueString() {
				matches = append(matches, t.ID)
			}
		}
		switch len(matches) {
		case 0:
			resp.Diagnostics.AddError("Template not found", fmt.Sprintf("No template named %q.", cfg.Name.ValueString()))
			return
		case 1:
			id = matches[0]
		default:
			resp.Diagnostics.AddError("Ambiguous template name",
				fmt.Sprintf("%d templates are named %q (IDs %v); look the template up by id instead.", len(matches), cfg.Name.ValueString(), matches))
			return
		}
	}

	t, err := d.client.GetTemplate(ctx, id)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "template", err)
		return
	}
	subject := types.StringNull()
	if t.Type == templateTypeTx {
		subject = types.StringValue(t.Subject)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, templateDataModel{
		ID:        types.Int64Value(t.ID),
		Name:      types.StringValue(t.Name),
		Type:      types.StringValue(t.Type),
		Subject:   subject,
		Body:      types.StringValue(t.Body),
		IsDefault: types.BoolValue(t.IsDefault),
	})...)
}

// ---------------------------------------------------------------------------
// listmonk_user_role
// ---------------------------------------------------------------------------

var _ datasource.DataSource = &userRoleDataSource{}

// NewUserRoleDataSource returns the listmonk_user_role data source.
func NewUserRoleDataSource() datasource.DataSource { return &userRoleDataSource{} }

type userRoleDataSource struct {
	client *client.Client
}

func (d *userRoleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_role"
}

func (d *userRoleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a listmonk user role by exact name, e.g. the built-in `Super Admin`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{MarkdownDescription: "Name of the role.", Required: true},
			"id":   schema.Int64Attribute{MarkdownDescription: "Numeric ID of the role.", Computed: true},
			"permissions": schema.SetAttribute{
				MarkdownDescription: "Permissions of the role. Empty for Super Admin, which has every permission implicitly.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *userRoleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromDataSource(req, resp)
}

func (d *userRoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg userRoleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roles, err := d.client.GetUserRoles(ctx)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "user roles", err)
		return
	}
	for _, r := range roles {
		if r.Name == cfg.Name.ValueString() {
			perms := append([]string(nil), r.Permissions...)
			slices.Sort(perms)
			cfg.ID = types.Int64Value(r.ID)
			cfg.Permissions = stringSetValue(perms)
			resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
			return
		}
	}
	resp.Diagnostics.AddError("User role not found", fmt.Sprintf("No user role named %q.", cfg.Name.ValueString()))
}
