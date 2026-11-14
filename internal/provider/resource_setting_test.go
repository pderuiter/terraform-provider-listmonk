package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Settings are instance-wide and every write reloads listmonk, so these
// tests do not run in parallel.
func TestAccSetting(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "listmonk_setting" "site_name" {
  key   = "app.site_name"
  value = jsonencode("Terraform acceptance test")
}

resource "listmonk_setting" "blocklist" {
  key   = "privacy.domain_blocklist"
  value = jsonencode(["spam.example", "junk.example"])
}

resource "listmonk_setting" "tracking" {
  key   = "privacy.individual_tracking"
  value = jsonencode(true)
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("listmonk_setting.site_name", tfjsonpath.New("id"), knownvalue.StringExact("app.site_name")),
				},
			},
			{
				Config: `
resource "listmonk_setting" "site_name" {
  key   = "app.site_name"
  value = jsonencode("Terraform acceptance test, updated")
}

resource "listmonk_setting" "blocklist" {
  key   = "privacy.domain_blocklist"
  value = jsonencode([])
}

resource "listmonk_setting" "tracking" {
  key   = "privacy.individual_tracking"
  value = jsonencode(false)
}
`,
			},
			{
				ResourceName:      "listmonk_setting.site_name",
				ImportState:       true,
				ImportStateId:     "app.site_name",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccSettingSMTPMaskedPassword(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// The password comes back masked and listmonk adds fields such
				// as uuid; neither may cause a diff (the test framework fails
				// on a non-empty plan after apply).
				Config: `
resource "listmonk_setting" "smtp" {
  key = "smtp"
  value = jsonencode([{
    enabled         = true
    host            = "smtp.example.com"
    hello_hostname  = ""
    port            = 587
    auth_protocol   = "login"
    username        = "listmonk"
    password        = "s3cret-password"
    email_headers   = []
    max_conns       = 5
    max_msg_retries = 2
    idle_timeout    = "15s"
    wait_timeout    = "5s"
    tls_type        = "STARTTLS"
    tls_skip_verify = false
  }])
}
`,
			},
		},
	})
}

func TestAccSettingUnknownKey(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "listmonk_setting" "bogus" {
  key   = "app.does_not_exist"
  value = jsonencode("x")
}
`,
				ExpectError: regexp.MustCompile(`listmonk has no setting "app.does_not_exist"`),
			},
		},
	})
}
