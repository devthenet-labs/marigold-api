// Copyright 2026 DevTheNet Labs.
// SPDX-License-Identifier: MIT
'use strict';
const {test} = require('node:test');
const assert = require('node:assert/strict');
const {validate, parseID} = require('./guard.cjs');
const sha = 'a'.repeat(40);
const REPO_ID = 1402832070;
const OWNER_ID = 332591015;

function fixture(kind = 'runtime', event = 'pull_request') {
  const identity = {owner: 'devthenet-labs', name: 'example-app', repoID: REPO_ID, ownerID: OWNER_ID};
  const repo = {id: REPO_ID, owner: {id: OWNER_ID}, full_name: 'devthenet-labs/example-app', fork: false, default_branch: 'main'};
  const path = kind === 'runtime' ? '.github/workflows/ci.yml' : '.github/workflows/agent-image.yml';
  return [identity, repo, {repository: repo, head_repository: repo, status: 'completed', conclusion: 'success',
    head_sha: sha, workflow_id: 5, path, event, head_branch: 'main', run_attempt: 1, pull_requests: [{number: 3}]},
  {id: 5, path, state: 'active'}, {number: 3, state: 'open', base: {ref: 'main', repo}, head: {repo, sha}},
  [{id: 7, name: `${kind}-${sha}-1`, expired: false, size_in_bytes: 123}], kind];
}

test('same-repo current PR, main runtime, dispatched main runtime and main agent accepted', () => {
  for (const [args, source] of [[fixture(), 'pull_request'], [fixture('runtime', 'push'), 'main'],
    [fixture('runtime', 'workflow_dispatch'), 'main'], [fixture('agent', 'push'), 'main'],
    [fixture('agent', 'workflow_dispatch'), 'main']]) {
    const result = validate(...args);
    assert.deepEqual(result, {artifact_id: '7', sha, kind: args[6], source});
  }
});

for (const [name, mutate] of Object.entries({
  fork: a => {a[2].head_repository = {...a[1], id: 888, fork: true};},
  reused_name: a => {a[1].id++;},
  wrong_owner: a => {a[1].owner.id++;},
  renamed_but_unconfigured: a => {a[1].full_name = 'devthenet-labs/other';},
  unset_repository_id: a => {a[0].repoID = NaN;},
  zero_owner_id: a => {a[0].ownerID = 0;},
  failed_run: a => {a[2].conclusion = 'failure';},
  in_progress: a => {a[2].status = 'in_progress';},
  wrong_workflow: a => {a[3].path = '.github/workflows/evil.yml';},
  inactive_workflow: a => {a[3].state = 'disabled_manually';},
  stale_head: a => {a[4].head.sha = 'b'.repeat(40);},
  closed_pr: a => {a[4].state = 'closed';},
  fork_pr: a => {a[4].head.repo = {...a[1], fork: true, id: 888};},
  missing_pr: a => {a[2].pull_requests = [];},
  ambiguous_pr: a => {a[2].pull_requests.push({number: 9});},
  wrong_base: a => {a[4].base.ref = 'other';},
  unsafe_sha: a => {a[2].head_sha = '$(id)';},
  old_attempt: a => {a[2].run_attempt = 2;},
  missing_artifact: a => {a[5] = [];},
  duplicate_artifact: a => {a[5].push({...a[5][0]});},
  expired_artifact: a => {a[5][0].expired = true;},
  huge_artifact: a => {a[5][0].size_in_bytes = 2 ** 32;},
  target_event: a => {a[2].event = 'pull_request_target';},
  wrong_main: a => {a[2].event = 'push'; a[2].head_branch = 'other';},
  agent_pr: a => {a[6] = 'agent'; a[3].path = a[2].path = '.github/workflows/agent-image.yml';},
  unknown_kind: a => {a[6] = 'constructor';},
})) {
  test(`reject ${name}`, () => {const args = fixture(); mutate(args); assert.throws(() => validate(...args));});
}

test('agent images come only from main runs of the agent workflow', () => {
  const args = fixture('agent', 'push');
  args[2].path = args[3].path = '.github/workflows/ci.yml';
  assert.throws(() => validate(...args), /workflow identity/);
});

test('repository variables must be canonical numeric IDs', () => {
  assert.equal(parseID('1402832070', 'X'), 1402832070);
  for (const bad of [undefined, '', '0', '01', '-1', '1e9', ' 1', '1 ', '12345678901234567', 'abc']) {
    assert.throws(() => parseID(bad, 'X'), /must be a numeric ID/, String(bad));
  }
});
