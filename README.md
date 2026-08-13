# Mission Control demo application

A deliberately small Go and PostgreSQL application for demonstrating how Mission Control correlates source, deployment, database, and runtime changes during incident diagnosis.

The application exposes a message API and applies embedded-in-image SQL migrations with [goose](https://github.com/pressly/goose). Migration `00002` intentionally renames the `body` column while the application still depends on it, producing a Kubernetes crash loop for incident diagnosis.

## Architecture

```text
GitHub repository / Actions
           |
Kubernetes Deployment -> Go API -> PostgreSQL
           |                         |-- public.messages
           |                         `-- goose_db_version
           `---- Mission Control relationship graph ----'
```

The application applies every pending migration and verifies its required schema before opening the HTTP listener. A migration or schema incompatibility therefore terminates the process and gives Kubernetes a clear failure signal.

## Run locally

```sh
docker compose up --build
curl http://localhost:8080/api/messages
curl -X POST http://localhost:8080/api/messages \
  -H 'Content-Type: application/json' \
  -d '{"text":"hello from the demo"}'
```

Or run PostgreSQL separately and start the app with:

```sh
DATABASE_URL='postgres://demo:demo@localhost:5432/demo?sslmode=disable' go run ./cmd/server
```

## Deploy to Kubernetes

After this directory is pushed to the public `adityathebe/mc-demo` repository, images from `main` are published as both `ghcr.io/adityathebe/mc-demo:latest` and an immutable full Git SHA tag.

```sh
kubectl apply -k deploy/kubernetes
kubectl -n mc-demo rollout status statefulset/postgres
kubectl -n mc-demo rollout status deployment/mc-demo-app
kubectl -n mc-demo port-forward service/mc-demo-app 8080:80
```

For a deterministic deployment, replace `:latest` and both `REPLACE_WITH_GIT_SHA` annotation values in `deploy/kubernetes/app.yaml` with the full SHA produced by GitHub Actions. Keep the repository annotation: the Mission Control relationship scraper uses its stable external ID.

## Install Mission Control scrapers

The manifests expect Mission Control CRDs and controllers in the cluster and use the `mission-control` namespace. Change `clusterName` in `kubernetes-scraper.yaml` if desired. The GitHub Actions scraper references `connection://mission-control/github`; point that field at a GitHub connection available in your installation.

Apply the application first so its source Secret exists, then copy the lab credentials and install the scraper resources:

```sh
kubectl apply -k deploy/kubernetes
kubectl apply -k deploy/mission-control
```

They install:

- a Kubernetes scraper scoped to `mc-demo`, including Kubernetes Events;
- public GitHub repository and GitHub Actions scrapers;
- a PostgreSQL connection;
- a custom SQL scraper for table schemas and goose migration history;
- a `ScrapePlugin` that establishes cross-scraper relationships.

The SQL scraper creates `Postgres::Table` and `Postgres::Migration` config items. Goose rows emit timestamped `MigrationApplied` changes. The plugin connects:

```text
Kubernetes::Deployment -> GitHub::Repository
Postgres::Table --------> GitHub::Repository
Postgres::Migration ----> GitHub::Repository
Postgres::Migration ----> Postgres::Table/public.messages
```

`secret-copy.yaml` duplicates the lab-only credentials because Kubernetes Secrets are namespace-scoped. Keep it in sync with `deploy/kubernetes/secret.yaml`; replace this pattern with your secret synchronization mechanism outside a lab.

## Validate

```sh
go test ./...
go vet ./...
kubectl kustomize deploy/kubernetes >/dev/null
kubectl kustomize deploy/mission-control >/dev/null
```
