package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/pderuiter/terraform-provider-listmonk/internal/client"
)

func TestAccRolesAndUsers(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "listmonk_list" "news" {
  name = "%[1]s-news"
}

resource "listmonk_user_role" "editor" {
  name        = "%[1]s-editor"
  permissions = ["campaigns:get_all", "templates:get"]
}

resource "listmonk_list_role" "news" {
  name = "%[1]s-news"

  list {
    list_id     = listmonk_list.news.id
    permissions = ["list:get"]
  }
}

resource "listmonk_user" "person" {
  username     = "%[1]s-person"
  email        = "%[1]s@example.com"
  user_role_id = listmonk_user_role.editor.id
  list_role_id = listmonk_list_role.news.id
}

resource "listmonk_user" "bot" {
  username     = "%[1]s-bot"
  type         = "api"
  user_role_id = listmonk_user_role.editor.id
}

data "listmonk_user_role" "editor" {
  name       = listmonk_user_role.editor.name
  depends_on = [listmonk_user_role.editor]
}
`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("listmonk_user.person", tfjsonpath.New("api_token"), knownvalue.Null()),
					statecheck.ExpectKnownValue("listmonk_user.person", tfjsonpath.New("name"), knownvalue.StringExact(name+"-person")),
					statecheck.ExpectKnownValue("listmonk_user.bot", tfjsonpath.New("password_login"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue("listmonk_user.bot", tfjsonpath.New("api_token"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.listmonk_user_role.editor", tfjsonpath.New("permissions"), knownvalue.SetSizeExact(2)),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.listmonk_user_role.editor", "id", "listmonk_user_role.editor", "id"),
					testAccCheckAPIUserToken("listmonk_user.bot"),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "listmonk_list" "news" {
  name = "%[1]s-news"
}

resource "listmonk_list" "events" {
  name = "%[1]s-events"
}

resource "listmonk_user_role" "editor" {
  name        = "%[1]s-editor2"
  permissions = ["campaigns:get_all", "campaigns:manage_all", "templates:get"]
}

resource "listmonk_list_role" "news" {
  name = "%[1]s-news2"

  list {
    list_id     = listmonk_list.news.id
    permissions = ["list:get", "list:manage"]
  }

  list {
    list_id     = listmonk_list.events.id
    permissions = ["list:get"]
  }
}

resource "listmonk_user" "person" {
  username       = "%[1]s-person"
  name           = "Test Person"
  email          = "%[1]s@example.com"
  status         = "disabled"
  password_login = true
  password       = "correct-horse-battery"
  user_role_id   = listmonk_user_role.editor.id
}

resource "listmonk_user" "bot" {
  username     = "%[1]s-bot"
  type         = "api"
  user_role_id = listmonk_user_role.editor.id
}
`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("listmonk_user_role.editor", tfjsonpath.New("permissions"), knownvalue.SetSizeExact(3)),
					statecheck.ExpectKnownValue("listmonk_list_role.news", tfjsonpath.New("list"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue("listmonk_user.person", tfjsonpath.New("list_role_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue("listmonk_user.person", tfjsonpath.New("status"), knownvalue.StringExact("disabled")),
					// The token survives updates.
					statecheck.ExpectKnownValue("listmonk_user.bot", tfjsonpath.New("api_token"), knownvalue.NotNull()),
				},
			},
			{
				ResourceName:      "listmonk_user_role.editor",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "listmonk_list_role.news",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:            "listmonk_user.person",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password", "api_token"},
			},
			{
				ResourceName:            "listmonk_user.bot",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"api_token"},
			},
		},
	})
}

// testAccCheckAPIUserToken checks that the exported token really
// authenticates against listmonk.
func testAccCheckAPIUserToken(addr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("%s not in state", addr)
		}
		c, err := client.New(client.Config{
			BaseURL:    os.Getenv(envURL),
			Username:   rs.Primary.Attributes["username"],
			Token:      rs.Primary.Attributes["api_token"],
			MaxRetries: -1,
		})
		if err != nil {
			return err
		}
		return c.Health(context.Background())
	}
}

func TestAccUserValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "listmonk_user" "test" {
  username     = "someone"
  user_role_id = 1
}
`,
				ExpectError: regexp.MustCompile(`Users of type "user" need an e-mail address`),
				PlanOnly:    true,
			},
			{
				Config: `
resource "listmonk_user" "test" {
  username     = "bot"
  type         = "api"
  email        = "bot@example.com"
  user_role_id = 1
}
`,
				ExpectError: regexp.MustCompile(`E-mail not allowed for API users`),
				PlanOnly:    true,
			},
			{
				Config: `
resource "listmonk_user" "test" {
  username       = "someone"
  email          = "someone@example.com"
  password_login = true
  user_role_id   = 1
}
`,
				ExpectError: regexp.MustCompile(`password_login = true needs a password`),
				PlanOnly:    true,
			},
		},
	})
}
