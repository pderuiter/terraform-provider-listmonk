# Campaign templates must render the campaign body with {{ template "content" . }}.
resource "listmonk_template" "branded" {
  name = "Branded"
  type = "campaign"
  body = file("${path.module}/templates/branded.html")
}

# Transactional templates have a fixed subject and are sent with POST /api/tx.
resource "listmonk_template" "welcome" {
  name    = "Welcome"
  type    = "tx"
  subject = "Welcome, {{ .Subscriber.Name }}"
  body    = <<-HTML
    <p>Hi {{ .Subscriber.Name }},</p>
    <p>Thanks for signing up.</p>
  HTML
}
