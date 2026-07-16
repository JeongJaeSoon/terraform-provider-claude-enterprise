package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// NewMembersDataSource registers claude-enterprise_members.
func NewMembersDataSource() datasource.DataSource {
	return &membersDataSource{}
}

type membersDataSource struct {
	data *providerData
}

type memberModel struct {
	UserID            types.String `tfsdk:"user_id"`
	Email             types.String `tfsdk:"email"`
	Name              types.String `tfsdk:"name"`
	EffectiveAmount   types.String `tfsdk:"effective_amount"`
	Currency          types.String `tfsdk:"currency"`
	Period            types.String `tfsdk:"period"`
	SourceType        types.String `tfsdk:"source_type"`
	SpendLimitID      types.String `tfsdk:"spend_limit_id"`
	PeriodToDateSpend types.String `tfsdk:"period_to_date_spend"`
}

type membersModel struct {
	ByEmail  map[string]memberModel `tfsdk:"by_email"`
	ByUserID map[string]memberModel `tfsdk:"by_user_id"`
}

func (d *membersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_members"
}

func memberAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"user_id":              schema.StringAttribute{Computed: true, MarkdownDescription: "Member user ID (`user_01...`)."},
		"email":                schema.StringAttribute{Computed: true, MarkdownDescription: "Member email address (lower-cased)."},
		"name":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Member display name."},
		"effective_amount":     schema.StringAttribute{Computed: true, MarkdownDescription: "Effective spend limit in minor units (cents). Null means unlimited; `\"0\"` means included usage only."},
		"currency":             schema.StringAttribute{Computed: true, MarkdownDescription: "Billing currency code, e.g. `USD`."},
		"period":               schema.StringAttribute{Computed: true, MarkdownDescription: "Limit period, currently `monthly`."},
		"source_type":          schema.StringAttribute{Computed: true, MarkdownDescription: "Where the limit resolved from: `user`, `seat_tier`, `rbac_group`, or `organization` (open set)."},
		"spend_limit_id":       schema.StringAttribute{Computed: true, MarkdownDescription: "ID of the spend limit row the effective limit resolved from."},
		"period_to_date_spend": schema.StringAttribute{Computed: true, MarkdownDescription: "Spend accrued this period, informational only (may temporarily read \"0\")."},
	}
}

func (d *membersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	memberObject := schema.NestedAttributeObject{Attributes: memberAttributes()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every current organization member with their effective spend limit. " +
			"Use `by_email` to resolve emails to user IDs. Deleted members are excluded.",
		Attributes: map[string]schema.Attribute{
			"by_email": schema.MapNestedAttribute{
				Computed:            true,
				NestedObject:        memberObject,
				MarkdownDescription: "Members keyed by lower-cased email address.",
			},
			"by_user_id": schema.MapNestedAttribute{
				Computed:            true,
				NestedObject:        memberObject,
				MarkdownDescription: "Members keyed by user ID.",
			},
		},
	}
}

func (d *membersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.data = data
}

func (d *membersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	rows, err := d.data.Client.ListEffectiveSpendLimits(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list effective spend limits", err.Error())
		return
	}

	state := membersModel{
		ByEmail:  map[string]memberModel{},
		ByUserID: map[string]memberModel{},
	}
	for _, row := range rows {
		if row.Actor.Deleted {
			continue
		}
		email := strings.ToLower(row.Actor.EmailAddress)
		m := memberModel{
			UserID:            types.StringValue(row.Actor.UserID),
			Email:             types.StringValue(email),
			Name:              types.StringValue(row.Actor.Name),
			EffectiveAmount:   types.StringPointerValue(row.Amount),
			Currency:          types.StringValue(row.Currency),
			Period:            types.StringValue(row.Period),
			SourceType:        types.StringValue(row.Source.Type),
			SpendLimitID:      types.StringValue(row.SpendLimitID),
			PeriodToDateSpend: types.StringValue(row.PeriodToDateSpend),
		}
		state.ByEmail[email] = m
		state.ByUserID[row.Actor.UserID] = m
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
