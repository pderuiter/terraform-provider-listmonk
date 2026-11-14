package provider

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

var (
	_ resource.Resource                = &userRoleResource{}
	_ resource.ResourceWithImportState = &userRoleResource{}
)

// superAdminRoleID is the built-in Super Admin role, which listmonk does not
// allow to be changed or deleted.
const superAdminRoleID = 1

// NewUserRoleResource returns the listmonk_user_role resource.
func NewUserRoleResource() resource.Resource { return &userRoleResource{} }

type userRoleResource struct {
	client *client.Client
}

type userRoleModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Permissions types.Set    `tfsdk:"permissions"`
}

func (r *userRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_role"
}

func (r *userRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a listmonk user role: a named set of global permissions assigned to users. " +
			"The built-in Super Admin role (ID 1) cannot be managed.",
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
			"permissions": schema.SetAttribute{
				MarkdownDescription: "Permissions, e.g. `campaigns:get_all`, `subscribers:manage`, `templates:get`. " +
					"The server rejects unknown permissions; see the listmonk documentation for the full list.",
				ElementType: types.StringType,
				Required:    true,
				Validators: []validator.Set{setvalidator.ValueStringsAre(
					stringvalidator.RegexMatches(permissionRegexp, "must look like <area>:<action>, e.g. lists:get_all"),
				)},
			},
		},
	}
}

func (r *userRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromResource(req, resp)
}

func (r *userRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	perms := setToStrings(ctx, plan.Permissions, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.CreateUserRole(ctx, client.UserRole{Name: plan.Name.ValueString(), Permissions: perms})
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "create", "user role", err)
		return
	}

	plan.ID = types.Int64Value(out.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *userRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetUserRole(ctx, state.ID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "user role", err)
		return
	}

	state.Name = types.StringValue(out.Name)
	perms := append([]string(nil), out.Permissions...)
	sort.Strings(perms)
	state.Permissions = stringSetValue(perms)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *userRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state userRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	perms := setToStrings(ctx, plan.Permissions, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.UpdateUserRole(ctx, state.ID.ValueInt64(), client.UserRole{Name: plan.Name.ValueString(), Permissions: perms})
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "update", "user role", err)
		return
	}

	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *userRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteRole(ctx, state.ID.ValueInt64()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "delete", "user role", err)
	}
}

func (r *userRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importNumericID(ctx, req, resp)
	var id types.Int64
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, pathID, &id)...)
	if !resp.Diagnostics.HasError() && id.ValueInt64() == superAdminRoleID {
		resp.Diagnostics.AddError("Cannot import the Super Admin role",
			"listmonk does not allow the built-in Super Admin role (ID 1) to be changed, so it cannot be managed.")
	}
}
