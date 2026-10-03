# Copyright 2026 DevTheNet Labs.
# SPDX-License-Identifier: MIT
"""Pin the build/publish boundary as well as the data validators."""
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
KINDS = {"runtime": "AGENT", "agent": "RUNTIME"}  # kind -> the other kind's variable prefix


def workflow(name):
    return (ROOT / "workflows" / name).read_text()


def job_block(text, job):
    """The text of one top-level job in a workflow (jobs are indented two spaces)."""
    match = re.search(rf"^  {job}:\n(.*?)(?=^  [a-z][a-z0-9_-]*:\n|\Z)", text, re.M | re.S)
    if not match:
        raise AssertionError(f"job {job} not found")
    return match.group(1)


class WorkflowBoundaryTest(unittest.TestCase):
    def test_builds_have_no_oidc_credentials_or_publisher_configuration(self):
        for name in ["ci.yml", "agent-image.yml"]:
            text = workflow(name)
            for forbidden in ["id-token:", "secrets.", "vars.", "role-to-assume:", "docker login", "--push"]:
                self.assertNotIn(forbidden, text, f"{name}: {forbidden}")
            self.assertIn('test -z "${ACTIONS_ID_TOKEN_REQUEST_URL:-}"', text)
            self.assertIn("persist-credentials: false", text)

    def test_main_builds_are_never_cancelled(self):
        # Every main commit publishes the runtime image a preview of an
        # unchanged repository runs, so main runs get one group per commit
        # (never a shared ref group, where a newer push replaces a pending
        # run) and are never cancelled.
        text = workflow("ci.yml")
        self.assertIn("cancel-in-progress: ${{ github.event_name == 'pull_request' }}\n", text)
        self.assertIn(
            "group: test-${{ github.event_name == 'pull_request' && "
            "format('pr-{0}', github.event.pull_request.number) || github.sha }}\n", text)
        self.assertNotIn("github.ref }}", text.split("concurrency:", 1)[1].split("jobs:", 1)[0])

    def test_publishers_are_separate_trusted_workflows(self):
        for kind, other in KINDS.items():
            text = workflow(f"publish-{kind}.yml")
            upper = kind.upper()
            self.assertIn("ref: ${{ github.workflow_sha }}", text)
            self.assertIn("github.ref == 'refs/heads/main'", text)
            self.assertIn(f"role-to-assume: ${{{{ vars.{upper}_ROLE_ARN }}}}", text)
            self.assertIn(f"IMAGE_REPOSITORY: ${{{{ vars.{upper}_IMAGE_REPOSITORY }}}}", text)
            self.assertIn(f"IMAGE_KIND: {kind}\n", text)
            # A publisher never even names the other kind's role or repository.
            self.assertNotIn(f"vars.{other}_", text)
            self.assertNotIn("inputs.kind", text)
            self.assertNotIn("secrets.", text)
            self.assertNotIn("secrets:", text)
            for forbidden in ["docker run", "docker build", "head.sha", "npm install", "go test"]:
                self.assertNotIn(forbidden, text)
            # The configuration check runs before any credential is requested.
            self.assertLess(text.index("check-config.sh"), text.index("configure-aws-credentials"))
            self.assertLess(text.index("validate_oci.py"), text.index("configure-aws-credentials"))

    def test_runtime_and_agent_publishing_are_gated_separately(self):
        text = workflow("publish-images.yml")
        runtime, agent = job_block(text, "runtime"), job_block(text, "agent")
        self.assertEqual(text.count("vars.PREVIEW_PUBLISH_ENABLED == 'true'"), 1)
        self.assertEqual(text.count("vars.AGENT_PUBLISH_ENABLED == 'true'"), 1)
        self.assertIn("vars.PREVIEW_PUBLISH_ENABLED == 'true'", runtime)
        self.assertIn("uses: ./.github/workflows/publish-runtime.yml", runtime)
        self.assertIn("vars.AGENT_PUBLISH_ENABLED == 'true'", agent)
        self.assertIn("uses: ./.github/workflows/publish-agent.yml", agent)
        self.assertIn("github.event.workflow_run.event != 'pull_request'", agent)
        self.assertIn("workflows: [test, agent image]", text)
        self.assertNotIn("secrets: inherit", text)

    def test_no_workflow_runs_untrusted_code_with_a_privileged_trigger(self):
        for path in (ROOT / "workflows").glob("*.yml"):
            self.assertNotIn("pull_request_target", path.read_text(), path.name)


if __name__ == "__main__":
    unittest.main()
