package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const testCampaignBody = `<html><body>{{ template "content" . }}</body></html>`

func TestAccTemplateCampaign(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "listmonk_template" "test" {
  name = %q
  type = "campaign"
  body = %q
}
`, name, testCampaignBody),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("listmonk_template.test", tfjsonpath.New("is_default"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue("listmonk_template.test", tfjsonpath.New("subject"), knownvalue.Null()),
				},
			},
			{
				// An update makes listmonk store the name as subject; that
				// must not show up as drift on the next plan.
				Config: fmt.Sprintf(`
resource "listmonk_template" "test" {
  name = "%s-v2"
  type = "campaign"
  body = %q
}
`, name, "<div>"+testCampaignBody+"</div>"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("listmonk_template.test", plancheck.ResourceActionUpdate)},
				},
			},
			{
				ResourceName:      "listmonk_template.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Changing the type replaces the template.
				Config: fmt.Sprintf(`
resource "listmonk_template" "test" {
  name    = "%s-v2"
  type    = "tx"
  subject = "Hello {{ .Subscriber.Name }}"
  body    = "<p>Hi</p>"
}
`, name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("listmonk_template.test", plancheck.ResourceActionDestroyBeforeCreate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("listmonk_template.test", tfjsonpath.New("subject"), knownvalue.StringExact("Hello {{ .Subscriber.Name }}")),
				},
			},
		},
	})
}

func TestAccTemplateValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "listmonk_template" "test" {
  name = "x"
  type = "tx"
  body = "x"
}
`,
				ExpectError: regexp.MustCompile(`Transactional \(tx\) templates require a subject`),
				PlanOnly:    true,
			},
			{
				Config: `
resource "listmonk_template" "test" {
  name    = "x"
  type    = "campaign"
  subject = "x"
  body    = "x"
}
`,
				ExpectError: regexp.MustCompile(`Only tx templates have a subject`),
				PlanOnly:    true,
			},
		},
	})
}

func TestAccDefaultTemplate(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc")

	// Not parallel: there is one default template per instance.
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "listmonk_template" "builtin" {
  name = "Default campaign template"
}

resource "listmonk_template" "test" {
  name = %q
  type = "campaign"
  body = %q
}

resource "listmonk_default_template" "this" {
  template_id = listmonk_template.test.id
}
`, name, testCampaignBody),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("listmonk_default_template.this", "template_id", "listmonk_template.test", "id"),
					resource.TestCheckResourceAttr("data.listmonk_template.builtin", "type", "campaign"),
				),
			},
			{
				// Hand the default back to the built-in template so the test
				// template can be destroyed (listmonk refuses to delete the default).
				Config: fmt.Sprintf(`
data "listmonk_template" "builtin" {
  name = "Default campaign template"
}

resource "listmonk_template" "test" {
  name = %q
  type = "campaign"
  body = %q
}

resource "listmonk_default_template" "this" {
  template_id = data.listmonk_template.builtin.id
}
`, name, testCampaignBody),
				Check: resource.TestCheckResourceAttrPair("listmonk_default_template.this", "template_id", "data.listmonk_template.builtin", "id"),
			},
		},
	})
}
