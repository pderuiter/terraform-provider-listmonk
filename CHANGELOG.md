# Changelog

Release notes are generated from Conventional Commit messages and published
on each [GitHub release](https://github.com/pderuiter/terraform-provider-listmonk/releases).
This file records what those notes cannot: upgrade instructions and
behaviour changes users must act on.

## Unreleased

- `listmonk_setting` with `key = "smtp"` now rejects a server that leaves out
  a duration field the listmonk version has (`idle_timeout`, `wait_timeout`
  and, from listmonk v6, `msg_retry_delay`) or sets one that is not a
  duration. listmonk stores a missing duration as `""` on the next full
  settings save and then fails to start. On listmonk v6, add
  `msg_retry_delay` (e.g. `"10s"`) to every SMTP server before upgrading.

## 0.1.0 (unreleased)

First release.

- Resources: `listmonk_list`, `listmonk_template`, `listmonk_default_template`,
  `listmonk_subscriber`, `listmonk_user`, `listmonk_user_role`,
  `listmonk_list_role`, `listmonk_setting`.
- Data sources: `listmonk_list`, `listmonk_lists`, `listmonk_template`,
  `listmonk_user_role`.
- Tested against listmonk v5.1.0 and v6.2.0 with Terraform 1.13 and 1.17.
