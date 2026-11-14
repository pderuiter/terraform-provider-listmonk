resource "listmonk_setting" "site_name" {
  key   = "app.site_name"
  value = jsonencode("Example newsletter")
}

resource "listmonk_setting" "root_url" {
  key   = "app.root_url"
  value = jsonencode("https://listmonk.example.com")
}

resource "listmonk_setting" "from_email" {
  key   = "app.from_email"
  value = jsonencode("Example <newsletter@example.com>")
}

resource "listmonk_setting" "individual_tracking" {
  key   = "privacy.individual_tracking"
  value = jsonencode(false)
}

# The SMTP password is write-only: listmonk masks it when read back.
resource "listmonk_setting" "smtp" {
  key = "smtp"
  value = jsonencode([{
    enabled         = true
    host            = "smtp.example.com"
    hello_hostname  = ""
    port            = 587
    auth_protocol   = "login"
    username        = "listmonk"
    password        = var.smtp_password
    email_headers   = []
    max_conns       = 10
    max_msg_retries = 2
    idle_timeout    = "15s"
    wait_timeout    = "5s"
    tls_type        = "STARTTLS"
    tls_skip_verify = false
  }])
}

variable "smtp_password" {
  type      = string
  sensitive = true
}
