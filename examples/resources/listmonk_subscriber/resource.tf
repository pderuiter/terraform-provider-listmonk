resource "listmonk_list" "internal" {
  name = "Internal seed list"
}

# A seed address that receives every campaign sent to the internal list.
resource "listmonk_subscriber" "seed" {
  email                    = "seed@example.com"
  name                     = "Seed inbox"
  lists                    = [listmonk_list.internal.id]
  preconfirm_subscriptions = true

  attributes = jsonencode({
    team = "marketing"
  })
}
