# Build and publication boundary

This is a benign, stateless demo app. It has no accounts, secrets, database, filesystem writes or outbound requests.

## Two images, two roles

| Image           | Built from       | Destination (repository variable)                      | Trusted reusable publisher |
| --------------- | ---------------- | ------------------------------------------------------ | -------------------------- |
| Runtime         | PR head, or main | `RUNTIME_IMAGE_REPOSITORY`:`sha-<40hex>` (+ `main-…`)  | `publish-runtime.yml`      |
| Agent toolchain | main only        | `AGENT_IMAGE_REPOSITORY`:`toolchain-v1`                | `publish-agent.yml`        |

Build jobs have only `contents: read`, no OIDC permission, no repository variables, no cloud or model credentials and
no persisted checkout credential. Fork PRs may test and build but never publish. The uploaded artifact is untrusted
data, not authority.

Publication runs on `workflow_run`, from the default branch's trusted workflow snapshot. It never checks out, builds or
runs PR source and never unpacks image layers. Before asking for AWS credentials it:

1. checks its repository variables (`check-config.sh`): an unset or malformed one fails closed;
2. re-reads the repository, run, workflow, PR and artifact metadata from GitHub's API (`guard.cjs`). The repository's
   identity is its immutable numeric ID and owner ID; a PR must still be open, in this exact repository, targeting
   main, at the successful run's exact head. The artifact is selected by ID from that specific run;
3. validates a bounded, single-image linux/amd64 OCI archive (`validate_oci.py`): blob hashes and descriptor sizes,
   no links, traversal, duplicates, foreign URLs or unreferenced blobs.

Only then does it assume its own role and copy the opaque blobs with the distribution's `skopeo copy
--preserve-digests` (`copy-image.sh`). Tags are immutable: a re-run with the same digest is a no-op, and a different
digest at an existing tag is a hard failure. Each tag is checked on its own, so a re-run of a main publish still adds
a missing `main-<sha>` tag.

AWS binds each role to this repository's immutable OIDC subject
(`repo:devthenet-labs@<owner id>/marigold-api@<repository id>:ref:refs/heads/main`), the numeric repository and owner
IDs, the `main` ref, audience `sts.amazonaws.com` and its **own** `job_workflow_ref`. The runtime publisher therefore
cannot assume the agent role, and neither role can write any other repository or reach Kubernetes, IAM or secrets.
Because of that, a wrong repository variable can only make a publish fail.

Changes to `.github/` and `.patchy/` need human review; patchy's intent changesets refuse both paths. These protections
assume main's maintainers stay trusted.

## Repository variables

| Variable                   | Meaning                                                                      |
| -------------------------- | ---------------------------------------------------------------------------- |
| `PREVIEW_PUBLISH_ENABLED`  | `true` to publish runtime (preview) images; anything else skips them         |
| `AGENT_PUBLISH_ENABLED`    | `true` to publish the agent toolchain image; independent of previews         |
| `PUBLISH_REPOSITORY_ID`    | this repository's numeric ID (`gh api repos/<owner>/<repo> --jq .id`)        |
| `PUBLISH_OWNER_ID`         | the owning organisation's numeric ID                                         |
| `AWS_REGION`               | the registry's region                                                        |
| `ECR_REGISTRY`             | `<account>.dkr.ecr.<region>.amazonaws.com`                                   |
| `RUNTIME_IMAGE_REPOSITORY` | the runtime ECR repository, e.g. `patchy/previews/<app>`                      |
| `RUNTIME_ROLE_ARN`         | the runtime publisher role                                                   |
| `AGENT_IMAGE_REPOSITORY`   | the agent ECR repository, e.g. `patchy/app-envs/<app>`                        |
| `AGENT_ROLE_ARN`           | the agent publisher role                                                     |

A skipped publisher is not a successful publication. To roll back, set the `*_PUBLISH_ENABLED` variables to anything
but `true`, then revoke the role trust policies. Retain published images unless their deletion is deliberately
approved.
