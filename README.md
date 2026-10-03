# marigold-api

A small, benign Go JSON API: the backend half of the Marigold demo, whose frontend is
[marigold-web](https://github.com/devthenet-labs/marigold-web). No accounts, storage, secrets or outbound requests,
and no dependencies beyond the Go standard library.

Routes on port 8080:

- `GET /api/greeting`: `{"message": "Hello from Marigold", "servedAt": "<RFC 3339, UTC>", "revision": "<commit>"}`.
- `GET /healthz`: `{"status":"ok"}`.
- Anything else: a JSON 404 (`{"error":"not found"}`); other methods get a JSON 405.

**Every API route lives under `/api`.** In a preview, one host serves both apps: the load balancer sends `/api/...` to
this service _without rewriting the path_, and everything else to marigold-web. The browser calls the API on the same
origin; the two services never call each other (preview pods cannot reach one another), so the API sends no CORS
headers.

## Develop

Use Go 1.26.6.

```sh
gofmt -l .
go vet ./...
go test -race ./...
go run .
node --test .github/publish/guard.test.cjs
python3 -m unittest discover -s .github/publish -p 'test_*.py'
```

The root `Dockerfile` builds the runtime image: a static binary on distroless `static-debian12:nonroot`, uid 65532, no
shell, listening on 8080. It writes nothing, so it runs with a read-only root filesystem:

```sh
docker build --build-arg BUILD_SHA="$(git rev-parse HEAD)" -t marigold-api:local .
docker run --rm --read-only --cap-drop ALL --security-opt no-new-privileges \
  -p 127.0.0.1:8080:8080 marigold-api:local
curl -s localhost:8080/api/greeting
```

`.patchy/Dockerfile` is a different image: patchy's agent base plus the pinned Go toolchain, offline
(`GOPROXY=off`), with no application source baked in. `.patchy/agent.yaml` names its immutable `toolchain-v1` tag, which
the agent publisher pushes from main.

## CI and images

- `test` (`ci.yml`) checks formatting, vets and race-tests the exact head, runs the publishers' guard tests and
  actionlint, and builds the runtime image as an OCI artifact **without credentials or OIDC**. PR runs supersede older
  runs of the same PR; main runs are never cancelled, so every main commit gets its image.
- `agent image` builds the toolchain image on main, also uncredentialed.
- `publish images` is the trusted, main-context publisher (`workflow_run`). It never runs PR code: it re-reads the run
  from GitHub's API, validates the OCI archive as data, and only then assumes a narrowly scoped role to copy it:
  - runtime: `sha-<commit>` for open same-repository PR heads and main commits; main commits are also tagged
    `main-<commit>` so the registry keeps them for previews of an unchanged repository;
  - agent: `toolchain-v1`, from main only.

See [SECURITY.md](SECURITY.md) for the trust boundary and the repository variables.
