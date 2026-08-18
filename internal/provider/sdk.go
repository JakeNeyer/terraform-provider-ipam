package provider

import (
	"fmt"
	"time"

	"github.com/JakeNeyer/ipam-go/ipam"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

const defaultListLimit = 500

func parseID(label, raw string) (uuid.UUID, diag.Diagnostics) {
	var diags diag.Diagnostics
	id, err := ipam.ParseUUID(raw)
	if err != nil {
		diags.AddError("Invalid "+label, fmt.Sprintf("%q is not a valid UUID: %s", raw, err))
	}
	return id, diags
}

func idString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func listOpts(name string) *ipam.ListOptions {
	opts := &ipam.ListOptions{Limit: defaultListLimit}
	if name != "" {
		opts.Name = name
	}
	return opts
}
