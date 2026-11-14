resource "listmonk_list" "newsletter" {
  name = "Newsletter"
}

resource "listmonk_list" "events" {
  name = "Events"
}

resource "listmonk_list_role" "newsletter_editors" {
  name = "Newsletter editors"

  list {
    list_id     = listmonk_list.newsletter.id
    permissions = ["list:get", "list:manage"]
  }

  list {
    list_id     = listmonk_list.events.id
    permissions = ["list:get"]
  }
}
