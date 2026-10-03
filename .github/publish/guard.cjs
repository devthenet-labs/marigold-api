// Copyright 2026 DevTheNet Labs.
// SPDX-License-Identifier: MIT
'use strict';

// The trusted publishers' gate: re-read the triggering run from GitHub's API
// and refuse anything that is not this repository's own successful build of
// an open same-repository PR head or of main. Nothing here trusts the
// workflow_run payload or the artifact's contents.
//
// The repository's identity is its immutable numeric IDs, from the
// repository variables PUBLISH_REPOSITORY_ID and PUBLISH_OWNER_ID. The name
// comes from the run's own context and only selects the API path; a
// repository later created under a reused name fails the ID check.

const SHA = /^[0-9a-f]{40}$/;
const ID = /^[1-9][0-9]{0,15}$/;
// Each image kind is built by exactly one uncredentialed workflow.
const SOURCE_WORKFLOW = {runtime: '.github/workflows/ci.yml', agent: '.github/workflows/agent-image.yml'};
const MAX_MIB = {runtime: 128, agent: 2048};

function check(ok, message) {
  if (!ok) throw new Error(message);
}

// Repository variables arrive as strings; anything but a canonical positive
// integer (including an unset variable) fails closed.
function parseID(value, name) {
  check(typeof value === 'string' && ID.test(value) && Number.isSafeInteger(Number(value)), `${name} must be a numeric ID`);
  return Number(value);
}

function validate(identity, repo, run, workflow, pr, artifacts, kind) {
  const {owner, name, repoID, ownerID} = identity;
  check(Number.isSafeInteger(repoID) && repoID > 0 && Number.isSafeInteger(ownerID) && ownerID > 0, 'configured identity');
  check(repo.id === repoID && repo.owner.id === ownerID && repo.full_name === `${owner}/${name}` &&
    !repo.fork && repo.default_branch === 'main', 'repository identity');
  check(run.repository.id === repoID && run.head_repository.id === repoID &&
    !run.head_repository.fork, 'forks must never publish');
  check(run.status === 'completed' && run.conclusion === 'success' && SHA.test(run.head_sha), 'successful exact head required');
  check(Number.isSafeInteger(run.run_attempt) && run.run_attempt > 0, 'run attempt');
  check(Object.hasOwn(SOURCE_WORKFLOW, kind), 'image kind');
  const path = SOURCE_WORKFLOW[kind];
  check(workflow.path === path && workflow.state === 'active' && workflow.id === run.workflow_id && run.path === path, 'workflow identity');
  let source;
  if (run.event === 'pull_request') {
    check(kind === 'runtime' && pr && run.pull_requests.length === 1 &&
      run.pull_requests[0].number === pr.number, 'PR association');
    check(pr.state === 'open' && pr.base.ref === 'main' && pr.base.repo.id === repoID &&
      pr.head.repo && pr.head.repo.id === repoID && !pr.head.repo.fork &&
      pr.head.sha === run.head_sha, 'open same-repository PR at current head required');
    source = 'pull_request';
  } else {
    check(['push', 'workflow_dispatch'].includes(run.event) && run.head_branch === 'main', 'main only');
    source = 'main';
  }
  const artifactName = `${kind}-${run.head_sha}-${run.run_attempt}`;
  const matches = artifacts.filter(a => a.name === artifactName && !a.expired);
  check(matches.length === 1, 'exactly one matching artifact required');
  const artifact = matches[0];
  check(Number.isSafeInteger(artifact.id) && artifact.id > 0 && artifact.size_in_bytes > 0 &&
    artifact.size_in_bytes <= MAX_MIB[kind] * 1024 * 1024, 'artifact bounds');
  return {artifact_id: String(artifact.id), sha: run.head_sha, kind, source};
}

async function guard({github, context, runID, kind, repositoryID, ownerID}) {
  check(/^[1-9][0-9]{0,18}$/.test(String(runID)), 'run ID');
  const identity = {
    owner: context.repo.owner,
    name: context.repo.repo,
    repoID: parseID(repositoryID, 'PUBLISH_REPOSITORY_ID'),
    ownerID: parseID(ownerID, 'PUBLISH_OWNER_ID'),
  };
  const params = {owner: identity.owner, repo: identity.name};
  const {data: repo} = await github.rest.repos.get(params);
  const {data: run} = await github.rest.actions.getWorkflowRun({...params, run_id: runID});
  const {data: workflow} = await github.rest.actions.getWorkflow({...params, workflow_id: run.workflow_id});
  let pr;
  if (run.event === 'pull_request' && run.pull_requests.length === 1) {
    ({data: pr} = await github.rest.pulls.get({...params, pull_number: run.pull_requests[0].number}));
  }
  // Per-run artifacts only; never search globally or trust metadata from the artifact.
  const artifacts = await github.paginate(github.rest.actions.listWorkflowRunArtifacts,
    {...params, run_id: runID, per_page: 100});
  return validate(identity, repo, run, workflow, pr, artifacts, kind);
}

module.exports = {guard, validate, parseID};
