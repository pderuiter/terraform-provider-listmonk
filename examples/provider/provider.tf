terraform {
  required_providers {
    listmonk = {
      source  = "pderuiter/listmonk"
      version = "~> 0.1"
    }
  }
}

# Credentials of a listmonk API user (Admin -> Users -> type "API").
# Prefer the LISTMONK_URL, LISTMONK_USERNAME and LISTMONK_API_TOKEN
# environment variables over putting the token in configuration.
provider "listmonk" {
  url       = "https://listmonk.example.com"
  username  = "terraform"
  api_token = var.listmonk_api_token
}

variable "listmonk_api_token" {
  type      = string
  sensitive = true
}
