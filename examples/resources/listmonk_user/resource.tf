resource "listmonk_user_role" "campaign_manager" {
  name        = "Campaign manager"
  permissions = ["campaigns:get_all", "campaigns:manage_all", "templates:get"]
}

# A person who logs in through OIDC (no password login).
resource "listmonk_user" "alex" {
  username     = "alex"
  name         = "Alex Example"
  email        = "alex@example.com"
  user_role_id = listmonk_user_role.campaign_manager.id
}

# An API user for an integration. The token is only known to Terraform for
# users it created; store it in a secret manager.
resource "listmonk_user" "crm_sync" {
  username     = "crm-sync"
  type         = "api"
  user_role_id = listmonk_user_role.campaign_manager.id
}

output "crm_sync_token" {
  value     = listmonk_user.crm_sync.api_token
  sensitive = true
}
