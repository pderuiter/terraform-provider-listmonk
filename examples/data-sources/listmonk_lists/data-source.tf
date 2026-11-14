data "listmonk_lists" "newsletters" {
  tag  = "newsletter"
  type = "public"
}

output "newsletter_list_ids" {
  value = data.listmonk_lists.newsletters.lists[*].id
}
