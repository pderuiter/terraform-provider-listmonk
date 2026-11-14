resource "listmonk_user_role" "campaign_manager" {
  name = "Campaign manager"
  permissions = [
    "campaigns:get_all",
    "campaigns:manage_all",
    "campaigns:send",
    "templates:get",
    "media:get",
    "media:manage",
  ]
}
