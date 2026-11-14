package provider

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

var pathID = path.Root("id")

// permissionRegexp matches listmonk permission names such as lists:get_all.
var permissionRegexp = regexp.MustCompile(`^[a-z_]+:[a-z_]+$`)

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

// clientFromResource extracts the API client in a resource's Configure.
func clientFromResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *client.Client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("Expected *client.Client, got %T. Please report this issue to the provider developers.", req.ProviderData))
		return nil
	}
	return c
}

// clientFromDataSource extracts the API client in a data source's Configure.
func clientFromDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *client.Client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("Expected *client.Client, got %T. Please report this issue to the provider developers.", req.ProviderData))
		return nil
	}
	return c
}

// importNumericID implements ImportState for resources identified by a
// numeric listmonk ID.
func importNumericID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || id <= 0 {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("Expected a positive numeric listmonk ID, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

func apiErrorDiag(d *diag.Diagnostics, action, object string, err error) {
	d.AddError(fmt.Sprintf("Unable to %s %s", action, object), err.Error())
}

// stringSetValue builds a types.Set of strings; nil and empty give an empty set.
func stringSetValue(values []string) types.Set {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValueMust(types.StringType, elems)
}

// stringListValue builds a types.List of strings; nil and empty give an empty list.
func stringListValue(values []string) types.List {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	return types.ListValueMust(types.StringType, elems)
}

func int64SetValue(values []int64) types.Set {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.Int64Value(v))
	}
	return types.SetValueMust(types.Int64Type, elems)
}

// setToStrings converts a known string set to a sorted slice.
func setToStrings(ctx context.Context, s types.Set, d *diag.Diagnostics) []string {
	out := []string{}
	if s.IsNull() || s.IsUnknown() {
		return out
	}
	d.Append(s.ElementsAs(ctx, &out, false)...)
	sort.Strings(out)
	return out
}

func listToStrings(ctx context.Context, l types.List, d *diag.Diagnostics) []string {
	out := []string{}
	if l.IsNull() || l.IsUnknown() {
		return out
	}
	d.Append(l.ElementsAs(ctx, &out, false)...)
	return out
}

func setToInt64s(ctx context.Context, s types.Set, d *diag.Diagnostics) []int64 {
	out := []int64{}
	if s.IsNull() || s.IsUnknown() {
		return out
	}
	d.Append(s.ElementsAs(ctx, &out, false)...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
