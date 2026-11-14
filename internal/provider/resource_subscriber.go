package provider

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
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
	_ resource.Resource                = &subscriberResource{}
	_ resource.ResourceWithImportState = &subscriberResource{}
)

// NewSubscriberResource returns the listmonk_subscriber resource.
func NewSubscriberResource() resource.Resource { return &subscriberResource{} }

type subscriberResource struct {
	client *client.Client
}

type subscriberModel struct {
	ID                      types.Int64          `tfsdk:"id"`
	UUID                    types.String         `tfsdk:"uuid"`
	Email                   types.String         `tfsdk:"email"`
	Name                    types.String         `tfsdk:"name"`
	Status                  types.String         `tfsdk:"status"`
	Attributes              jsontypes.Normalized `tfsdk:"attributes"`
	Lists                   types.Set            `tfsdk:"lists"`
	PreconfirmSubscriptions types.Bool           `tfsdk:"preconfirm_subscriptions"`
	SubscriptionStatus      types.Map            `tfsdk:"subscription_status"`
}

func (r *subscriberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscriber"
}

func (r *subscriberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a listmonk subscriber and its list memberships. Meant for a handful of " +
			"well-known addresses (seed, test or internal recipients), not for bulk audiences: use listmonk's " +
			"import for those.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "Numeric ID of the subscriber.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "UUID of the subscriber.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "E-mail address. listmonk stores it in lower case; case-only differences are not drift.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(3, 1000)},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the subscriber. When unset, listmonk derives one from the e-mail address.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "`enabled`, `disabled` or `blocklisted`. Defaults to `enabled`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("enabled"),
				Validators:          []validator.String{stringvalidator.OneOf("enabled", "disabled", "blocklisted")},
			},
			"attributes": schema.StringAttribute{
				MarkdownDescription: "Custom attributes as a JSON object, e.g. `jsonencode({ city = \"Utrecht\" })`.",
				CustomType:          jsontypes.NormalizedType{},
				Optional:            true,
			},
			"lists": schema.SetAttribute{
				MarkdownDescription: "IDs of the lists the subscriber belongs to. Lists not in this set are unsubscribed. " +
					"Defaults to no lists.",
				ElementType: types.Int64Type,
				Optional:    true,
				Computed:    true,
				Default:     setdefault.StaticValue(int64SetValue(nil)),
			},
			"preconfirm_subscriptions": schema.BoolAttribute{
				MarkdownDescription: "Confirm subscriptions to double opt-in lists without sending an opt-in e-mail. " +
					"Applies when lists are added. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"subscription_status": schema.MapAttribute{
				MarkdownDescription: "Subscription status per list ID (`unconfirmed`, `confirmed` or `unsubscribed`).",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (r *subscriberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromResource(req, resp)
}

func (r *subscriberResource) payload(ctx context.Context, m subscriberModel, d *diag.Diagnostics) client.SubscriberRequest {
	attribs := map[string]any{}
	if !m.Attributes.IsNull() && !m.Attributes.IsUnknown() {
		if err := json.Unmarshal([]byte(m.Attributes.ValueString()), &attribs); err != nil {
			d.AddError("Invalid attributes", "attributes must be a JSON object: "+err.Error())
		}
	}
	name := ""
	if !m.Name.IsUnknown() {
		name = m.Name.ValueString()
	}
	return client.SubscriberRequest{
		Email:                   m.Email.ValueString(),
		Name:                    name,
		Status:                  m.Status.ValueString(),
		Attribs:                 attribs,
		Lists:                   setToInt64s(ctx, m.Lists, d),
		PreconfirmSubscriptions: m.PreconfirmSubscriptions.ValueBool(),
	}
}

func (r *subscriberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan subscriberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.payload(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.CreateSubscriber(ctx, payload)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "create", "subscriber", err)
		return
	}

	subscriberToModel(out, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *subscriberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state subscriberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetSubscriber(ctx, state.ID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "subscriber", err)
		return
	}

	// Import starts from an ID only.
	if state.PreconfirmSubscriptions.IsNull() {
		state.PreconfirmSubscriptions = types.BoolValue(false)
	}
	subscriberToModel(out, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *subscriberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state subscriberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.payload(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueInt64()
	if _, err := r.client.UpdateSubscriber(ctx, id, payload); err != nil {
		apiErrorDiag(&resp.Diagnostics, "update", "subscriber", err)
		return
	}

	// Re-read: the update response does not reflect list changes on every
	// listmonk version.
	out, err := r.client.GetSubscriber(ctx, id)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read", "subscriber", err)
		return
	}
	subscriberToModel(out, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *subscriberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state subscriberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteSubscriber(ctx, state.ID.ValueInt64()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "delete", "subscriber", err)
	}
}

func (r *subscriberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importNumericID(ctx, req, resp)
}

func subscriberToModel(s *client.Subscriber, m *subscriberModel, d *diag.Diagnostics) {
	m.ID = types.Int64Value(s.ID)
	m.UUID = types.StringValue(s.UUID)
	if m.Email.IsNull() || !strings.EqualFold(m.Email.ValueString(), s.Email) {
		m.Email = types.StringValue(s.Email)
	}
	m.Name = types.StringValue(s.Name)
	m.Status = types.StringValue(s.Status)

	if len(s.Attribs) == 0 && m.Attributes.IsNull() {
		// Keep "not set" rather than writing {}.
	} else {
		b, err := json.Marshal(s.Attribs)
		if err != nil {
			d.AddError("Unable to encode subscriber attributes", err.Error())
			return
		}
		if s.Attribs == nil {
			b = []byte("{}")
		}
		m.Attributes = jsontypes.NewNormalizedValue(string(b))
	}

	ids := make([]int64, 0, len(s.Lists))
	statuses := make(map[string]attr.Value, len(s.Lists))
	for _, l := range s.Lists {
		ids = append(ids, l.ID)
		statuses[itoa(l.ID)] = types.StringValue(l.SubscriptionStatus)
	}
	m.Lists = int64SetValue(ids)
	m.SubscriptionStatus = types.MapValueMust(types.StringType, statuses)
}
