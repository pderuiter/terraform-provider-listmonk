package provider

import (
	"context"
	"encoding/json"
	"os"
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

// testAccSMTPConfig returns a listmonk_setting for one SMTP server. extra is
// added to the server object; the duration fields this listmonk version
// knows are always set.
func testAccSMTPConfig(t *testing.T, extra string) string {
	t.Helper()
	retry := ""
	if testAccHasMsgRetryDelay(t) {
		retry = `msg_retry_delay = "10s"`
	}
	return testAccSMTPConfigWith(retry + "\n    " + extra)
}

func testAccSMTPConfigWith(fields string) string {
	return `
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
    ` + fields + `
  }])
}
`
}

// testAccHasMsgRetryDelay reports whether the server's SMTP entries carry
// msg_retry_delay (listmonk v6 and later).
func testAccHasMsgRetryDelay(t *testing.T) bool {
	t.Helper()
	settings, err := testAccClient(t).GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var smtp []map[string]any
	if err := json.Unmarshal(settings["smtp"], &smtp); err != nil {
		t.Fatal(err)
	}
	for _, s := range smtp {
		if _, ok := s["msg_retry_delay"]; ok {
			return true
		}
	}
	return false
}

func TestAccSettingSMTPMaskedPassword(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test; set TF_ACC=1")
	}
	testAccPreCheck(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// The password comes back masked and listmonk adds fields such
				// as uuid; neither may cause a diff (the test framework fails
				// on a non-empty plan after apply).
				Config: testAccSMTPConfig(t, ""),
			},
		},
	})
}

// A duration listmonk knows but the configuration leaves out would be stored
// as "" by the next full settings save, after which listmonk fails to start.
func TestAccSettingSMTPMissingDuration(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test; set TF_ACC=1")
	}
	testAccPreCheck(t)
	if !testAccHasMsgRetryDelay(t) {
		t.Skip("listmonk before v6 has no msg_retry_delay")
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccSMTPConfigWith(""),
				ExpectError: regexp.MustCompile(`does not set msg_retry_delay`),
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
