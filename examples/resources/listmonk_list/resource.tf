resource "listmonk_list" "newsletter" {
  name        = "Newsletter"
  type        = "public"
  optin       = "double"
  tags        = ["newsletter", "marketing"]
  description = "Monthly product newsletter"
}
