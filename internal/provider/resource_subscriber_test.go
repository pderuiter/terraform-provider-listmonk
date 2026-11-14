package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccSubscriber(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc")
	email := name + "@example.com"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "listmonk_list" "single" {
  name = "%[1]s-single"
}

resource "listmonk_list" "double" {
  name  = "%[1]s-double"
  optin = "double"
}

resource "listmonk_subscriber" "test" {
  email                    = %[2]q
  lists                    = [listmonk_list.single.id, listmonk_list.double.id]
  preconfirm_subscriptions = true
  attributes = jsonencode({
    city = "Utrecht"
    plan = { tier = "pro", seats = 5 }
  })
}
`, name, email),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("listmonk_subscriber.test", tfjsonpath.New("status"), knownvalue.StringExact("enabled")),
					statecheck.ExpectKnownValue("listmonk_subscriber.test", tfjsonpath.New("lists"), knownvalue.SetSizeExact(2)),
					// listmonk derives a name from the address.
					statecheck.ExpectKnownValue("listmonk_subscriber.test", tfjsonpath.New("name"), knownvalue.NotNull()),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("listmonk_subscriber.test", "subscription_status.%", "listmonk_subscriber.test", "lists.#"),
				),
			},
			{
				// Upper-case e-mail is stored lower-case by listmonk: no diff.
				Config: fmt.Sprintf(`
resource "listmonk_list" "single" {
  name = "%[1]s-single"
}

resource "listmonk_list" "double" {
  name  = "%[1]s-double"
  optin = "double"
}

resource "listmonk_subscriber" "test" {
  email  = upper(%[2]q)
  name   = "Test Subscriber"
  status = "disabled"
  lists  = [listmonk_list.single.id]
  attributes = jsonencode({
    plan = { seats = 5, tier = "pro" }
    city = "Amsterdam"
  })
}
`, name, email),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("listmonk_subscriber.test", tfjsonpath.New("name"), knownvalue.StringExact("Test Subscriber")),
					statecheck.ExpectKnownValue("listmonk_subscriber.test", tfjsonpath.New("status"), knownvalue.StringExact("disabled")),
					statecheck.ExpectKnownValue("listmonk_subscriber.test", tfjsonpath.New("lists"), knownvalue.SetSizeExact(1)),
				},
			},
			{
				ResourceName:            "listmonk_subscriber.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"email"},
			},
		},
	})
}
