# Terraform Provider for listmonk

Manage a [listmonk](https://listmonk.app) instance as code: mailing lists, e-mail
templates, subscribers, users, user and list roles, and instance settings.

```terraform
terraform {
  required_providers {
    listmonk = {
      source  = "pderuiter/listmonk"
      version = "~> 0.1"
    }
  }
}

provider "listmonk" {
  url = "https://listmonk.example.com"
  # username and api_token from LISTMONK_USERNAME / LISTMONK_API_TOKEN
}

resource "listmonk_list" "newsletter" {
  name  = "Newsletter"
  type  = "public"
  optin = "double"
  tags  = ["newsletter"]
}
```

Documentation: [Terraform Registry](https://registry.terraform.io/providers/pderuiter/listmonk/latest/docs)
(generated from [`docs/`](docs/)).

| Resources                   | Data sources         |
|-----------------------------|----------------------|
| `listmonk_list`             | `listmonk_list`      |
| `listmonk_template`         | `listmonk_lists`     |
| `listmonk_default_template` | `listmonk_template`  |
| `listmonk_subscriber`       | `listmonk_user_role` |
| `listmonk_user`             |                      |
| `listmonk_user_role`        |                      |
| `listmonk_list_role`        |                      |
| `listmonk_setting`          |                      |

## Compatibility

| Component     | Tested                         |
|---------------|--------------------------------|
| listmonk      | v5.1.0, v6.2.0                 |
| Terraform CLI | 1.13, 1.17 (protocol 6)        |

Every change runs the full acceptance suite against each combination.

## Repository

Development happens on [gitlab.bsdserver.nl](https://gitlab.bsdserver.nl/wbyc/infra/iac/terraform/terraform-provider-listmonk);
[github.com/pderuiter/terraform-provider-listmonk](https://github.com/pderuiter/terraform-provider-listmonk)
is a read-only mirror that the Terraform Registry publishes from. Pull requests
on GitHub are not merged there; open an issue instead.

- [CONTRIBUTING.md](CONTRIBUTING.md): building, testing, docs, upgrading dependencies.
- [RELEASING.md](RELEASING.md): versioning, the release pipeline, signing keys.

## License

[Mozilla Public License 2.0](LICENSE), the licence of the HashiCorp
provider scaffolding.
