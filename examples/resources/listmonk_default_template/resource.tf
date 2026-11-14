resource "listmonk_template" "branded" {
  name = "Branded"
  type = "campaign"
  body = file("${path.module}/templates/branded.html")
}

# New campaigns use this template unless they pick another one.
resource "listmonk_default_template" "this" {
  template_id = listmonk_template.branded.id
}
