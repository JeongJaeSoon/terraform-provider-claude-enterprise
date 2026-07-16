package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"
)

// NewSpendLimitResource registers claude-enterprise_spend_limit.
func NewSpendLimitResource() resource.Resource {
	return &spendLimitResource{}
}

type spendLimitResource struct {
	data *providerData
}

type spendLimitModel struct {
	ID        types.String `tfsdk:"id"`
	UserID    types.String `tfsdk:"user_id"`
	UserEmail types.String `tfsdk:"user_email"`
	Amount    types.String `tfsdk:"amount"`
	Period    types.String `tfsdk:"period"`
	Currency  types.String `tfsdk:"currency"`
}

func (r *spendLimitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_spend_limit"
}

func (r *spendLimitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one member's per-user spend limit override. " +
			"Creating adopts any override that already exists for the same user and period (the API is an upsert); " +
			"deleting returns the member to their inherited limit.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Spend limit ID (`spl_...`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"user_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Member user ID (`user_01...`). Exactly one of `user_id` and `user_email` must be set. " +
					"Changing the target user replaces the resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_email": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Member email address, resolved to `user_id` at plan time. " +
					"Exactly one of `user_id` and `user_email` must be set. " +
					"If the same email later maps to a different user (member removed and re-invited), the resource is replaced.",
			},
			"amount": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Spend limit in minor units of the billing currency, as a string " +
					"(`\"50000\"` is 500.00 USD; `\"0\"` allows included usage only). Updated in place.",
			},
			"period": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("monthly"),
				MarkdownDescription: "Limit period. Currently only `monthly` is supported by the API.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"currency": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Billing currency code, e.g. `USD`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *spendLimitResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(path.MatchRoot("user_id"), path.MatchRoot("user_email")),
	}
}

func (r *spendLimitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.data = data
}

// resolveUserID returns the effective user ID from a (possibly unknown)
// planned user_id plus the configured email.
func (r *spendLimitResource) resolveUserID(ctx context.Context, plan spendLimitModel) (string, error) {
	if !plan.UserID.IsNull() && !plan.UserID.IsUnknown() {
		return plan.UserID.ValueString(), nil
	}
	return r.data.Resolver.UserIDByEmail(ctx, plan.UserEmail.ValueString())
}

func (r *spendLimitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan spendLimitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userID, err := r.resolveUserID(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("user_email"), "Cannot resolve user_email", err.Error())
		return
	}

	sl, err := r.data.Client.UpsertSpendLimit(ctx, userID, plan.Amount.ValueString(), plan.Period.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create spend limit", err.Error())
		return
	}

	plan.ID = types.StringValue(sl.ID)
	plan.UserID = types.StringValue(sl.Scope.UserID)
	plan.Amount = types.StringPointerValue(sl.Amount)
	plan.Period = types.StringValue(sl.Period)
	plan.Currency = types.StringValue(sl.Currency)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *spendLimitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state spendLimitModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sl, err := r.data.Client.GetSpendLimit(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read spend limit", err.Error())
		return
	}
	if sl.Scope.Type != "" && sl.Scope.Type != "user" {
		// Not a per-user override anymore; treat as gone.
		resp.State.RemoveResource(ctx)
		return
	}

	state.UserID = types.StringValue(sl.Scope.UserID)
	state.Amount = types.StringPointerValue(sl.Amount)
	state.Period = types.StringValue(sl.Period)
	state.Currency = types.StringValue(sl.Currency)
	// user_email is configuration identity the API cannot return; keep as-is.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *spendLimitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan spendLimitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userID, err := r.resolveUserID(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("user_email"), "Cannot resolve user_email", err.Error())
		return
	}

	sl, err := r.data.Client.UpsertSpendLimit(ctx, userID, plan.Amount.ValueString(), plan.Period.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update spend limit", err.Error())
		return
	}

	plan.ID = types.StringValue(sl.ID)
	plan.UserID = types.StringValue(sl.Scope.UserID)
	plan.Amount = types.StringPointerValue(sl.Amount)
	plan.Period = types.StringValue(sl.Period)
	plan.Currency = types.StringValue(sl.Currency)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *spendLimitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state spendLimitModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.data.Client.DeleteSpendLimit(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete spend limit", err.Error())
	}
}

func (r *spendLimitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by user_id is completed in the next task; spend_limit_id works now.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
