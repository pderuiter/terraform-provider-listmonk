# Contributing

## Layout

| Path                    | What                                                           |
|-------------------------|----------------------------------------------------------------|
| `internal/client/`      | listmonk REST client: auth, retries, typed models, API quirks  |
| `internal/provider/`    | Terraform Plugin Framework resources and data sources          |
| `examples/`             | HCL used in the docs (`resource.tf`, `data-source.tf`, `import.sh`) |
| `templates/`            | doc templates (`index.md.tmpl`); others are tfplugindocs defaults |
| `docs/`                 | **generated** Registry docs, committed                         |
| `tools/`                | separate module pinning tfplugindocs (`go tool tfplugindocs`)  |
| `scripts/`              | local/CI test environment and release scripts                  |

## Requirements

- Go (version from `go.mod`; the `toolchain` line is fetched automatically)
- Terraform CLI >= 1.13
- For acceptance tests: Docker, or a local PostgreSQL (`initdb`, `pg_ctl`)

## Everyday commands

```sh
make build        # go build
make test         # unit tests (no listmonk needed)
make lint         # golangci-lint (.golangci.yml)
make fmt          # gofmt + terraform fmt on examples/
make generate     # regenerate docs/ -- commit the result; CI fails on drift
make testacc      # acceptance tests, see below
```

## Acceptance tests

Acceptance tests (`TestAcc*`, gated on `TF_ACC=1`) create real objects in a
real listmonk. `scripts/testacc-local.sh` starts a throwaway one: it downloads
the listmonk release binary (checksum-verified), runs the installer with an API
user, and writes the credentials to `.testacc.env`.

```sh
# PostgreSQL in Docker (default)
LISTMONK_VERSION=v6.2.0 scripts/testacc-local.sh test

# Local PostgreSQL instead of Docker
PG_MODE=native LISTMONK_VERSION=v5.1.0 scripts/testacc-local.sh test

# One test, server kept running between runs
PG_MODE=native scripts/testacc-local.sh up
. ./.testacc.env && TF_ACC=1 go test ./internal/provider/ -run TestAccList -v
PG_MODE=native scripts/testacc-local.sh down
```

Never point the tests at a production listmonk: `TestAccSetting*` and
`TestAccDefaultTemplate` change instance-wide state.

CI runs every listmonk version × Terraform version in `test:acceptance`
(`.gitlab-ci.yml`). To support a new listmonk release, add it to that matrix,
run the suite locally against it first, and update the compatibility table in
`README.md` and `templates/index.md.tmpl`.

### Writing tests

- Every resource needs a test that creates, updates in place, imports
  (`ImportStateVerify`), and checks for an empty plan after each step (the
  framework does this by default).
- Behaviour that differs between listmonk versions is handled in the provider,
  not skipped in tests: both versions in the matrix run every test.
- Validation-only tests use `PlanOnly` and still need `TF_ACC` (they run the
  Terraform CLI).

## listmonk API notes

Learned the hard way; the reasoning lives next to the code that handles it.

- Not-found is `400` for lists, templates and subscribers, `404` for users
  (`client.IsNotFound` accepts both).
- Several endpoints accept and silently ignore what they cannot apply:
  unknown settings keys, `set-default` on a non-campaign template, clearing a
  list description. The provider re-reads after every write and fails instead
  of reporting success.
- `PUT /api/settings/{key}` exists from v6; on older versions the client
  rewrites the full settings object with masked secrets blanked (listmonk
  keeps the stored ones for blank values).
- An API user's token is returned once, by the create call, in `password`.

## Upgrading dependencies

Renovate opens MRs (`renovate.json`). The terraform-plugin-* libraries are
grouped because they must move together. After a framework upgrade:
`make generate` (schemas may render differently) and the full acceptance
matrix.

Go: CI builds with the base-go image (`BASE_GO_VERSION` in `.gitlab-ci.yml`);
keep `go.mod`'s `toolchain` line at most that image's Go version. Keep the `go` directive at the oldest Go that HashiCorp's
libraries support, not the newest.

## Commits

Conventional Commits; the version of the next release is derived from them
(see `RELEASING.md`). MR titles count: squash merges use them as the commit.
