data "listmonk_user_role" "super_admin" {
  name = "Super Admin"
}

resource "listmonk_user" "admin" {
  username     = "ops"
  email        = "ops@example.com"
  user_role_id = data.listmonk_user_role.super_admin.id
}
