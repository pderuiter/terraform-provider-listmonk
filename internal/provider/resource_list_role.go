package provider

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

var (
	_ resource.Resource                = &listRoleResource{}
	_ resource.ResourceWithImportState = &listRoleResource{}
)

// NewListRoleResource returns the listmonk_list_role resource.
func NewListRoleResource() resource.Resource { return &listRoleResource{} }

type listRoleResource struct {
	client *client.Client
}

type listRoleModel struct {
	ID    types.Int64  `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Lists types.Set    `tfsdk:"list"`
}

type listPermissionModel struct {
	ListID      types.Int64 `tfsdk:"list_id"`
	Permissions types.Set   `tfsdk:"permissions"`
}

var listPermissionAttrTypes = map[string]attr.Type{
	"list_id":     types.Int64Type,
	"permissions": types.SetType{ElemType: types.StringType},
}

func (r *listRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_list_role"
}

func (r *listRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a listmonk list role: per-list permissions that can be assigned to users " +
			"in addition to their user role.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "Numeric ID of the role.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the role.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 2000)},
			},
		},
		Blocks: map[string]schema.Block{
			"list": schema.SetNestedBlock{
				MarkdownDescription: "Permissions on one list. Repeat the block for each list.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"list_id": schema.Int64Attribute{
							MarkdownDescription: "ID of the list.",
							Required:            true,
						},
						"permissions": schema.SetAttribute{
							MarkdownDescription: "`list:get` and/or `list:manage`.",
							ElementType:         types.StringType,
							Required:            true,
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(1),
								setvalidator.ValueStringsAre(stringvalidator.OneOf("list:get", "list:manage")),
							},
						},
					},
				},
			},
		},
	}
}

func (r *listRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromResource(req, resp)
}

func listRolePayload(ctx context.Context, m listRoleModel, d *diag.Diagnostics) client.ListRole {
	out := client.ListRole{Name: m.Name.ValueString(), Lists: []client.ListPermission{}}
	if m.Lists.IsNull() || m.Lists.IsUnknown() {
		return out
	}
	var perms []listPermissionModel
	d.Append(m.Lists.ElementsAs(ctx, &perms, false)...)
	for _, p := range perms {
		out.Lists = append(out.Lists, client.ListPermission{
			ID:          p.ListID.ValueInt64(),
			Permissions: setToStrings(ctx, p.Permissions, d),
		})
	}
	return out
}

func (r *listRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan listRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := listRolePayload(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.CreateListRole(ctx, payload)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "create", "list role", err)
		return
	}

	plan.ID = types.Int64Value(out.ID)
	if plan.Lists.IsNull() || plan.Lists.IsUnknown() {
		plan.Lists = types.SetValueMust(types.ObjectType{AttrTypes: listPermissionAttrTypes}, []attr.Value{})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *listRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state listRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetListRole(ctx, state.ID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "list role", err)
		return
	}

	state.Name = types.StringValue(out.Name)
	state.Lists = listPermissionsValue(out.Lists, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *listRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state listRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := listRolePayload(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateListRole(ctx, state.ID.ValueInt64(), payload); err != nil {
		apiErrorDiag(&resp.Diagnostics, "update", "list role", err)
		return
	}

	plan.ID = state.ID
	if plan.Lists.IsNull() || plan.Lists.IsUnknown() {
		plan.Lists = types.SetValueMust(types.ObjectType{AttrTypes: listPermissionAttrTypes}, []attr.Value{})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *listRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state listRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteRole(ctx, state.ID.ValueInt64()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "delete", "list role", err)
	}
}

func (r *listRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importNumericID(ctx, req, resp)
}

func listPermissionsValue(perms []client.ListPermission, d *diag.Diagnostics) types.Set {
	objType := types.ObjectType{AttrTypes: listPermissionAttrTypes}
	elems := make([]attr.Value, 0, len(perms))
	for _, p := range perms {
		// A list role without per-list rows comes back as [{"id": null, ...}].
		if p.ID == 0 {
			continue
		}
		ps := append([]string(nil), p.Permissions...)
		sort.Strings(ps)
		obj, diags := types.ObjectValue(listPermissionAttrTypes, map[string]attr.Value{
			"list_id":     types.Int64Value(p.ID),
			"permissions": stringSetValue(ps),
		})
		d.Append(diags...)
		elems = append(elems, obj)
	}
	set, diags := types.SetValue(objType, elems)
	d.Append(diags...)
	return set
}
