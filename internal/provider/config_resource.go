package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/TGNThump/terraform-provider-vyos/internal/vyos"
	"github.com/foltik/vyos-client-go/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &ConfigResource{}
var _ resource.ResourceWithImportState = &ConfigResource{}
var _ resource.ResourceWithConfigure = &ConfigResource{}

func NewConfigResource() resource.Resource {
	return &ConfigResource{}
}

// ConfigResource defines the resource implementation.
type ConfigResource struct {
	vyosConfig *vyos.VyosConfig
}

// ConfigResourceModel describes the resource data model.
type ConfigResourceModel struct {
	Path  types.String  `tfsdk:"path"`
	Value types.Dynamic `tfsdk:"value"`
	Id    types.String  `tfsdk:"id"`
}

func (r *ConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (r *ConfigResource) Schema(ctx context.Context, request resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Configuration Resource",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Configuration identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"path": schema.StringAttribute{
				MarkdownDescription: "Configuration path",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.DynamicAttribute{
				MarkdownDescription: "JSON configuration for the path",
				Optional:            true,
				// PlanModifiers: []planmodifier.String{
				// 	NormalizeValue(),
				// },
				PlanModifiers: []planmodifier.Dynamic{
					NormalizeValue(),
				},
			},
		},
	}
}

func (r *ConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	vyosConfig, ok := req.ProviderData.(*vyos.VyosConfig)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.vyosConfig = vyosConfig
}

func (r *ConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *ConfigResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Check if config already exists
	tflog.Info(ctx, "Reading path "+data.Path.ValueString())

	components := strings.Split(data.Path.ValueString(), " ")
	parentPath := strings.Join(components[0:len(components)-1], " ")
	terminal := components[len(components)-1]

	parent, err := r.vyosConfig.Show(ctx, parentPath)
	if err != nil {
		resp.Diagnostics.AddError("No", err.Error())
		return
	}

	if parent != nil {
		existing := parent.(map[string]any)[terminal]

		if existing != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Configuration path '%s' already exists, try a resource import instead.", data.Path.ValueString()), fmt.Sprintf("%v", existing))
			return
		}
	}

	tflog.Info(ctx, "Setting path "+data.Path.ValueString()+" to value "+data.Value.UnderlyingValue().String())

	err = r.vyosConfig.Set(ctx, data.Path.ValueString(), data.Value.UnderlyingValue())
	if err != nil {
		resp.Diagnostics.AddError("No", err.Error())
		return
	}

	data.Id = types.StringValue(data.Path.ValueString())

	tflog.Info(ctx, "Set path "+data.Path.ValueString()+" to value "+data.Value.UnderlyingValue().String())

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *ConfigResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Error(ctx, "Reading path "+data.Path.ValueString())

	components := strings.Split(data.Path.ValueString(), " ")
	parentPath := strings.Join(components[0:len(components)-1], " ")
	terminal := components[len(components)-1]

	parent, err := r.vyosConfig.Show(ctx, parentPath)
	if err != nil {
		resp.Diagnostics.AddError("No", err.Error())
		return
	}

	if parent == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	config := parent.(map[string]any)[terminal]

	if config == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	tflog.Error(ctx, "TEST")
	jsonString, err := json.MarshalIndent(config, "", "  ")
	tflog.Error(ctx, string(jsonString))

	// Based on https://developer.hashicorp.com/terraform/plugin/framework/handling-data/types/dynamic#setting-values
	var value attr.Value
	// tftypes.NewValue()
	switch v := config.(type) {
	case bool:
		// value = tftypes.NewValue(tftypes.Bool, v)
		value = types.BoolValue(v)
	case string:
		value = types.StringValue(v)
	case int32:
		value = types.Int32Value(v)
	case int64:
		value = types.Int64Value(v)
	case float32:
		value = types.Float32Value(v)
	case float64:
		value = types.Float64Value(v)
	case []any:
		value, _ = types.ListValueFrom(ctx, types.DynamicType, v)
	case map[string]any:
		value, _ = types.MapValueFrom(ctx, types.DynamicType, v)
	// default:
	// 	value = v.(attr.Value)
	// 	value = types.MapValueMust(attr.Value, v.(map[string]attr.Value))
	}
	data.Value = types.DynamicValue(value)

	tflog.Error(ctx, "Read path "+data.Path.ValueString()+" with value "+data.Value.UnderlyingValue().String())

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *ConfigResourceModel
	var state *ConfigResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Updating path "+plan.Path.ValueString()+" to value "+plan.Value.UnderlyingValue().String())

	var payload []map[string]any

	payload = append(payload, map[string]any{
		"op":   "delete",
		"path": strings.Split(plan.Path.ValueString(), " "),
	})

	{
		flat, err := client.Flatten(plan.Value.UnderlyingValue())
		if err != nil {
			resp.Diagnostics.AddError("No", err.Error())
			return
		}

		for _, pair := range flat {
			subpath, value := pair[0], pair[1]

			prefixpath := plan.Path.ValueString()
			if len(prefixpath) > 0 && len(subpath) > 0 {
				prefixpath += " "
			}
			prefixpath += subpath

			payload = append(payload, map[string]any{
				"op":    "set",
				"path":  strings.Split(prefixpath, " "),
				"value": value,
			})
		}
	}

	tflog.Info(ctx, fmt.Sprintf("%v", payload))

	_, err := r.vyosConfig.ApiRequest(ctx, "configure", payload)
	if err != nil {
		resp.Diagnostics.AddError("No", err.Error())
		return
	}

	tflog.Info(ctx, "Updated path "+plan.Path.ValueString()+" to value "+plan.Value.UnderlyingValue().String())

	// Save updated plan into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *ConfigResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Deleting path "+data.Path.ValueString())

	err := r.vyosConfig.Delete(ctx, data.Path.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("No", err.Error())
		return
	}

	tflog.Info(ctx, "Deleted path "+data.Path.ValueString())
}

func (r *ConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), req.ID)...)
}

func getAttributeType(value any) attr.Type {
	switch v := value.(type) {
	case bool:
		return types.BoolType
	case string:
		return types.StringType
	case int32:
		return types.NumberType
	case int64:
		return types.Int64Type
	case float32:
		return types.Float32Type
	case float64:
		return types.Float64Type
	case []any:
		return types.ListType.WithElementType(types.ListType{}, getAttributeType(v))
	case map[string]any:
		return types.MapType.WithElementType(types.MapType{}, types.DynamicType)
	}
	return types.StringType
}


type normalizeValueModifier struct{}

func NormalizeValue() planmodifier.Dynamic {
	return normalizeValueModifier{}
}

func (m normalizeValueModifier) Description(_ context.Context) string {
	return "Normalize all VyOS config values passed in."
}

func (m normalizeValueModifier) MarkdownDescription(_ context.Context) string {
	return "Normalize all VyOS config values passed in."
}

func (m normalizeValueModifier) PlanModifyDynamic(ctx context.Context, req planmodifier.DynamicRequest, resp *planmodifier.DynamicResponse) {
	// Do nothing if there is no state value.
	if req.StateValue.IsNull() {
		return
	}

	// Do nothing if there is an unknown configuration value, otherwise interpolation gets messed up.
	if req.ConfigValue.IsUnknown() {
		return
	}

	// Do nothing if there is an unknown configuration value, otherwise interpolation gets messed up.
	if req.ConfigValue.IsUnknown() {
		return
	}

	// Normalize value and pass back in response
	resp.PlanValue = normalizeValue(ctx, req.PlanValue.UnderlyingValue()).(types.Dynamic)
	tflog.Error(ctx, resp.PlanValue.String())
}

func normalizeList(ctx context.Context, list []any) any {
	newList := []any{}
	for _, value := range list {
		if value != nil {
			newList = append(newList, normalizeValue(ctx, value))
		}
	}

	// Convert single-element lists to scalar value
	if len(newList) == 1 {
		return newList[0]
	}
	return newList
}

func normalizeObject(ctx context.Context, obj map[string]any) map[string]any {
	newObj := make(map[string]any)

	for key, value := range obj {
		if value != nil {
			(newObj)[key] = normalizeValue(ctx, value)
		}
	}

	return newObj
}

func normalizeValue(ctx context.Context, value any) any {
	switch v := value.(type) {
	case nil:
		return make(map[string]any)
	case []any:
		fmt.Println("Normalizing list: ", v)
		return normalizeList(ctx, v)
	case map[string]any:
		fmt.Println("Normalizing map: ", v)
		return normalizeObject(ctx, v)
	default:
		fmt.Println("Unhandled type: ", reflect.TypeOf(v))
	}
	return value
}
