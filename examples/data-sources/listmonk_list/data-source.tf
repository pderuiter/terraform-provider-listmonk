# Look up a list that is not managed by Terraform, e.g. one created in the UI.
data "listmonk_list" "default" {
  name = "Default list"
}

output "default_list_subscribers" {
  value = data.listmonk_list.default.subscriber_count
}
