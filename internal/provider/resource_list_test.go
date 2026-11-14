package provider

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

func TestAccList(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckListDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "listmonk_list" "test" {
  name = %q
}
`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("type"), knownvalue.StringExact("private")),
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("optin"), knownvalue.StringExact("single")),
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("status"), knownvalue.StringExact("active")),
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("tags"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("uuid"), knownvalue.NotNull()),
				},
			},
			{
				Config: fmt.Sprintf(`
resource "listmonk_list" "test" {
  name        = "%s-renamed"
  type        = "public"
  optin       = "double"
  tags        = ["newsletter", "tf"]
  description = "Managed by Terraform"
}
`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("name"), knownvalue.StringExact(name+"-renamed")),
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("optin"), knownvalue.StringExact("double")),
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("tags"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("newsletter"), knownvalue.StringExact("tf"),
					})),
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("description"), knownvalue.StringExact("Managed by Terraform")),
				},
			},
			{
				ResourceName:      "listmonk_list.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Removing description keeps it (listmonk cannot clear it) and
				// must not produce a diff.
				Config: fmt.Sprintf(`
resource "listmonk_list" "test" {
  name  = "%s-renamed"
  type  = "public"
  optin = "double"
  tags  = []
}
`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("description"), knownvalue.StringExact("Managed by Terraform")),
					statecheck.ExpectKnownValue("listmonk_list.test", tfjsonpath.New("tags"), knownvalue.SetSizeExact(0)),
				},
			},
		},
	})
}

func testAccCheckListDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "listmonk_list" {
				continue
			}
			id, _ := strconv.ParseInt(rs.Primary.ID, 10, 64)
			if _, err := c.GetList(context.Background(), id); !client.IsNotFound(err) {
				return fmt.Errorf("list %d still exists (err=%v)", id, err)
			}
		}
		return nil
	}
}

func TestAccListDataSources(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc")
	tag := acctest.RandomWithPrefix("tag")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "listmonk_list" "a" {
  name = "%[1]s-a"
  tags = [%[2]q]
}

resource "listmonk_list" "b" {
  name = "%[1]s-b"
  type = "public"
  tags = [%[2]q]
}

data "listmonk_list" "by_name" {
  name = listmonk_list.a.name
}

data "listmonk_list" "by_id" {
  id = listmonk_list.b.id
}

data "listmonk_lists" "tagged" {
  tag        = %[2]q
  depends_on = [listmonk_list.a, listmonk_list.b]
}

data "listmonk_lists" "tagged_public" {
  tag        = %[2]q
  type       = "public"
  depends_on = [listmonk_list.a, listmonk_list.b]
}
`, name, tag),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.listmonk_list.by_name", "id", "listmonk_list.a", "id"),
					resource.TestCheckResourceAttr("data.listmonk_list.by_id", "type", "public"),
					resource.TestCheckResourceAttr("data.listmonk_lists.tagged", "lists.#", "2"),
					resource.TestCheckResourceAttr("data.listmonk_lists.tagged_public", "lists.#", "1"),
					resource.TestCheckResourceAttrPair("data.listmonk_lists.tagged_public", "lists.0.id", "listmonk_list.b", "id"),
				),
			},
		},
	})
}
