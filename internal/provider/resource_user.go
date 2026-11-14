package provider

import (
	"context"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

var (
	_ resource.Resource                   = &userResource{}
	_ resource.ResourceWithImportState    = &userResource{}
	_ resource.ResourceWithValidateConfig = &userResource{}
)

const (
	userTypeUser = "user"
	userTypeAPI  = "api"
)

// Same rule as listmonk's cmd/users.go reUsername.
var usernameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_\-\.@]+$`)

// NewUserResource returns the listmonk_user resource.
func NewUserResource() resource.Resource { return &userResource{} }

type userResource struct {
	client *client.Client
}

type userModel struct {
	ID            types.Int64  `tfsdk:"id"`
	Username      types.String `tfsdk:"username"`
	Name          types.String `tfsdk:"name"`
	Type          types.String `tfsdk:"type"`
	Email         types.String `tfsdk:"email"`
	Status        types.String `tfsdk:"status"`
	PasswordLogin types.Bool   `tfsdk:"password_login"`
	Password      types.String `tfsdk:"password"`
	UserRoleID    types.Int64  `tfsdk:"user_role_id"`
	ListRoleID    types.Int64  `tfsdk:"list_role_id"`
	APIToken      types.String `tfsdk:"api_token"`
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a listmonk user: a person who logs in to the admin UI (`type = \"user\"`) " +
			"or an API user (`type = \"api\"`). For API users the generated token is exported as `api_token`; " +
			"listmonk shows it only once, at creation, so it is only available for users created by Terraform " +
			"(not for imported ones).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "Numeric ID of the user.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Login name: at least 3 characters out of letters, digits and `_ - . @`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(3, 2000),
					stringvalidator.RegexMatches(usernameRegexp, "may only contain letters, digits and _ - . @"),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name. Defaults to the username.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "`user` or `api`. Defaults to `user`. Changing it replaces the user.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(userTypeUser),
				Validators:          []validator.String{stringvalidator.OneOf(userTypeUser, userTypeAPI)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "E-mail address. Required for `type = \"user\"`; not used for API users.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "`enabled` or `disabled`. Defaults to `enabled`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("enabled"),
				Validators:          []validator.String{stringvalidator.OneOf("enabled", "disabled")},
			},
			"password_login": schema.BoolAttribute{
				MarkdownDescription: "Allow logging in with a password (instead of only OIDC). Defaults to `false`. " +
					"Not available for API users.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password (at least 8 characters) when `password_login` is enabled. listmonk " +
					"never returns it, so changes made outside Terraform are not detected.",
				Optional:   true,
				Sensitive:  true,
				Validators: []validator.String{stringvalidator.LengthAtLeast(8)},
			},
			"user_role_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the user role (`listmonk_user_role`, or 1 for Super Admin).",
				Required:            true,
			},
			"list_role_id": schema.Int64Attribute{
				MarkdownDescription: "ID of an optional list role (`listmonk_list_role`).",
				Optional:            true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "API token of an API user, generated by listmonk at creation. Null for " +
					"`type = \"user\"` and for imported API users.",
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *userResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg userModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Type.IsUnknown() {
		return
	}

	isAPI := cfg.Type.ValueString() == userTypeAPI
	if isAPI {
		if !cfg.Email.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("email"), "E-mail not allowed for API users",
				"API users have no e-mail address. Remove the email argument.")
		}
		if cfg.PasswordLogin.ValueBool() || !cfg.Password.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("password_login"), "Password not allowed for API users",
				"API users authenticate with their generated token. Remove password_login and password.")
		}
		return
	}

	if cfg.Email.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("email"), "Missing e-mail",
			"Users of type \"user\" need an e-mail address.")
	}
	if !cfg.PasswordLogin.IsUnknown() && cfg.PasswordLogin.ValueBool() && cfg.Password.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("password"), "Missing password",
			"password_login = true needs a password.")
	}
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromResource(req, resp)
}

func userPayload(m userModel, sendPassword bool) client.User {
	u := client.User{
		Username:      m.Username.ValueString(),
		Type:          m.Type.ValueString(),
		Status:        m.Status.ValueString(),
		PasswordLogin: m.PasswordLogin.ValueBool(),
		UserRoleID:    m.UserRoleID.ValueInt64(),
	}
	if !m.Name.IsUnknown() {
		u.Name = m.Name.ValueString()
	}
	if u.Type != userTypeAPI && !m.Email.IsUnknown() {
		u.Email = m.Email.ValueString()
	}
	if !m.ListRoleID.IsNull() && !m.ListRoleID.IsUnknown() {
		id := m.ListRoleID.ValueInt64()
		u.ListRoleID = &id
	}
	if sendPassword && !m.Password.IsNull() && !m.Password.IsUnknown() {
		p := m.Password.ValueString()
		u.Password = &p
	}
	return u
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.CreateUser(ctx, userPayload(plan, true))
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "create", "user", err)
		return
	}

	if plan.Type.ValueString() == userTypeAPI && out.Password != nil {
		plan.APIToken = types.StringValue(*out.Password)
	} else {
		plan.APIToken = types.StringNull()
	}
	userToModel(out, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetUser(ctx, state.ID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "user", err)
		return
	}

	userToModel(out, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only send the password when it changed: sending it also ends every
	// session of the user.
	sendPassword := !plan.Password.Equal(state.Password)
	out, err := r.client.UpdateUser(ctx, state.ID.ValueInt64(), userPayload(plan, sendPassword))
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "update", "user", err)
		return
	}

	plan.ID = state.ID
	plan.APIToken = state.APIToken
	userToModel(out, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteUser(ctx, state.ID.ValueInt64()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "delete", "user", err)
	}
}

func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importNumericID(ctx, req, resp)
}

func userToModel(u *client.User, m *userModel) {
	m.ID = types.Int64Value(u.ID)
	m.Username = types.StringValue(u.Username)
	m.Name = types.StringValue(u.Name)
	m.Type = types.StringValue(u.Type)
	// listmonk lower-cases e-mail addresses; a case-only difference is not drift.
	switch {
	case u.Email == "":
		// API users have no e-mail on current listmonk versions.
		m.Email = types.StringNull()
	case m.Email.IsNull() || m.Email.IsUnknown() || !strings.EqualFold(m.Email.ValueString(), u.Email):
		m.Email = types.StringValue(u.Email)
	}
	m.Status = types.StringValue(u.Status)
	// API users authenticate with their token; listmonk nevertheless reports
	// password_login = true for them, which is meaningless.
	m.PasswordLogin = types.BoolValue(u.PasswordLogin && u.Type != userTypeAPI)

	userRole, listRole := u.RoleIDs()
	m.UserRoleID = types.Int64Value(userRole)
	if listRole == nil {
		m.ListRoleID = types.Int64Null()
	} else {
		m.ListRoleID = types.Int64Value(*listRole)
	}
	if m.APIToken.IsUnknown() {
		m.APIToken = types.StringNull()
	}
}
