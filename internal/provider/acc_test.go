package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories is used for acceptance tests.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"ipam": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck skips acceptance tests unless TF_ACC=1 and IPAM_ENDPOINT, IPAM_TOKEN are set.
// Token must not contain double quotes (") to avoid breaking HCL config.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run acceptance tests")
	}
	if testing.Short() {
		t.Skip("skipping acceptance test in short mode")
	}
	if v := os.Getenv("IPAM_ENDPOINT"); v == "" {
		t.Skip("set IPAM_ENDPOINT to run acceptance tests")
	}
	if v := os.Getenv("IPAM_TOKEN"); v == "" {
		t.Skip("set IPAM_TOKEN to run acceptance tests")
	}
}

// testAccAllocationPreCheck skips tests that require allocation GET to work (create + read by id).
// Some IPAM API deployments return "not found" for GET /api/allocations/{id} after create (e.g. org scoping).
// Set IPAM_RUN_ALLOCATION_TESTS=1 to run these tests when your API supports allocation GET by id.
func testAccAllocationPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("IPAM_RUN_ALLOCATION_TESTS") != "1" {
		t.Skip("set IPAM_RUN_ALLOCATION_TESTS=1 to run allocation acceptance tests (requires API GET /api/allocations/{id} to return created allocations)")
	}
}

func testAccProviderConfig(endpoint, token string) string {
	return `
provider "ipam" {
  endpoint = "` + endpoint + `"
  token   = "` + token + `"
}
`
}
