"""Run with python3 -m unittest discover -s scripts/update -p 'test_*.py'."""
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("executor", Path(__file__).with_name("executor.py"))
executor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(executor)


class RecoveryTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.token = self.root / "token"
        self.token.write_text("enu_" + "a" * 64)
        self.token.chmod(0o600)
        self.config = self.root / "config.json"
        self.cfg = {"url": "http://127.0.0.1:1314", "instance_id": "1" * 32,
                    "token_file": str(self.token), "state_dir": str(self.root / "state"),
                    "backup_dir": str(self.root / "backups"), "platform": "linux/amd64",
                    "mode": "docker", "container": "isolated-app", "mounts": [],
                    "image_file": str(self.root / "image.env"), "docker": {}}
        self.config.write_text(json.dumps(self.cfg))
        self.config.chmod(0o600)
        self.ex = executor.Executor(self.config)
        self.ex.preflight = lambda: {"Id": "old-container", "Image": "sha256:" + "c" * 64}
        self.ex.runtime = lambda: {"instance_id": self.cfg["instance_id"], "revision": "1" * 40}
        self.task = {"id": "a" * 32, "status": "claimed", "channel": "edge",
                     "target_image": executor.IMAGE, "target_digest": "sha256:" + "b" * 64,
                     "target_revision": "2" * 40}
        self.calls = []
        self.ex.api = self.api

    def api(self, method, path, body=None, token=None):
        self.calls.append((path, body))
        return copy.deepcopy(self.task) if path.endswith("claim") else None

    def claim(self):
        self.ex.claim()
        return self.ex.record

    def test_claim_then_reload_does_not_claim_another_task(self):
        self.claim()
        self.ex.load_record()
        self.assertEqual(self.ex.record["task"]["id"], self.task["id"])
        self.assertEqual(sum(p.endswith("claim") for p, _ in self.calls), 1)

    def test_instance_mismatch_prevents_claim(self):
        self.ex.runtime = lambda: {"instance_id": "different"}
        with self.assertRaises(executor.Stop):
            self.ex.claim()
        self.assertEqual(self.calls, [])

    def test_claim_response_loss_retries_same_server_task(self):
        normal = self.api
        def lost(method, path, body=None, token=None):
            if not self.calls:
                self.calls.append((path, body))
                raise executor.Stop("http_transport")
            return normal(method, path, body, token)
        self.ex.api = lost
        with self.assertRaises(executor.Stop):
            self.ex.claim()
        self.assertFalse(self.ex.journal.exists())
        self.claim()
        self.assertEqual(self.ex.record["task"]["id"], self.task["id"])

    def test_reporting_loss_retains_order_and_retries_only_first_unconfirmed(self):
        self.claim()
        self.ex.queue("downloading")
        self.ex.queue("stopping")
        def lost(*args, **kwargs):
            raise executor.Stop("http_transport")
        self.ex.api = lost
        with self.assertRaises(executor.Stop):
            self.ex.flush()
        self.assertEqual(self.ex.record["pending"], ["downloading", "stopping"])
        self.ex.api = self.api
        self.ex.flush()
        self.assertEqual([body["status"] for _, body in self.calls if body], ["downloading", "stopping"])
        self.assertEqual(self.ex.record["confirmed"], "stopping")

    def test_409_keeps_evidence_and_never_drops_event(self):
        self.claim()
        self.ex.queue("downloading")
        self.ex.api = lambda *a, **k: (_ for _ in ()).throw(executor.Stop("http_409"))
        with self.assertRaises(executor.Stop):
            self.ex.flush()
        self.assertEqual(self.ex.record["pending"], ["downloading"])

    def test_final_retry_needs_no_claim_or_runtime_after_rotation(self):
        self.claim()
        self.ex.record["step"] = "complete"
        self.ex.queue("succeeded")
        self.ex.save()
        self.ex.runtime = lambda: self.fail("terminal retry called runtime")
        self.ex.run()
        self.assertTrue(self.ex.record["closed"])
        self.assertEqual(sum(p.endswith("claim") for p, _ in self.calls), 1)

    def test_record_instance_and_config_mismatch_rejected(self):
        self.claim()
        self.ex.record["instance_id"] = "wrong"
        self.ex.save()
        with self.assertRaises(executor.Stop):
            self.ex.load_record()

    def test_product_requires_u4_before_stop(self):
        self.claim()
        self.ex.download = lambda: {"image_id": "new", "manifest_digest": self.task["target_digest"]}
        self.ex.stop_container = lambda: self.fail("stopped without U4")
        with self.assertRaisesRegex(executor.Stop, "u4_backup_unavailable"):
            self.ex.run()

    def test_uncertain_stop_is_not_reexecuted(self):
        self.claim()
        self.ex.record["step"] = "stop_intent"
        self.ex.save()
        self.ex.stop_container = lambda: self.fail("repeated stop")
        with self.assertRaises(executor.Stop):
            self.ex.run()
        self.assertEqual(self.ex.record["step"], "attention")

    def test_target_already_started_only_verifies_and_reports(self):
        self.claim()
        self.ex.record["step"] = "replace_intent"
        self.ex.record["download"] = {"image_id": "new", "manifest_digest": self.task["target_digest"]}
        self.ex.save()
        self.ex.target_running = lambda: True
        self.ex.verify = lambda: None
        self.ex.replace = lambda: self.fail("repeated replacement")
        self.ex.run()
        self.assertTrue(self.ex.record["closed"])

    def test_pinned_reference_validation(self):
        for field, value in [("target_image", "evil/image"), ("target_digest", "edge-mcp"),
                             ("target_revision", "short"), ("id", "../escape"), ("channel", "latest")]:
            with self.subTest(field=field):
                self.task[field] = value
                with self.assertRaises(executor.Stop):
                    self.ex.claim()
                self.task = {"id": "a" * 32, "status": "claimed", "channel": "edge",
                             "target_image": executor.IMAGE, "target_digest": "sha256:" + "b" * 64,
                             "target_revision": "2" * 40}

    def test_space_check_happens_before_stop(self):
        self.claim()
        self.ex.cfg["min_free_bytes"] = 10**30
        with self.assertRaisesRegex(executor.Stop, "disk_space"):
            self.ex.check_space()

    def test_revoked_token_cannot_clear_pending_final(self):
        self.claim()
        self.ex.record["step"] = "complete"
        self.ex.queue("succeeded")
        self.ex.api = lambda *a, **k: (_ for _ in ()).throw(executor.Stop("http_401"))
        with self.assertRaises(executor.Stop):
            self.ex.run()
        self.assertFalse(self.ex.record["closed"])
        self.assertEqual(self.ex.record["pending"], ["succeeded"])

    def test_atomic_image_file_preserves_other_settings(self):
        path = Path(self.cfg["image_file"])
        path.write_text("# deployment\nOTHER=value\nUPDATE_IMAGE=old\n")
        executor.write_image(path, "pinned")
        self.assertEqual(path.read_text(), "# deployment\nOTHER=value\nUPDATE_IMAGE=pinned\n")

    def test_idle_claim_has_no_record(self):
        self.ex.api = lambda *a, **k: None
        self.ex.claim()
        self.assertIsNone(self.ex.record)
        self.assertFalse(self.ex.journal.exists())

    def test_nonclaimed_server_task_without_record_cannot_install(self):
        self.task["status"] = "replacing"
        with self.assertRaisesRegex(executor.Stop, "claimed_task_without_local_evidence"):
            self.ex.claim()
        self.assertEqual(self.ex.record["step"], "attention")
        with self.assertRaisesRegex(executor.Stop, "manual_reconciliation_required"):
            self.ex.run()

    def test_token_only_travels_on_curl_stdin_and_real_dto_code(self):
        import subprocess
        raw = self.token.read_text()
        def curl(args, **kwargs):
            self.assertNotIn(raw, " ".join(args))
            self.assertIn("Authorization: Bearer " + raw, kwargs["input"])
            self.assertNotIn("--location", args)
            return subprocess.CompletedProcess(args, 0, '{"code":1,"data":{"instance_id":"' + self.cfg["instance_id"] + '"}}\n200', '')
        with patch.object(executor.subprocess, "run", side_effect=curl):
            self.assertEqual(executor.Executor(self.config).runtime()["instance_id"], self.cfg["instance_id"])

    def test_registry_index_platform_and_image_id_are_distinct(self):
        self.claim()
        requested = self.task["target_digest"]
        platform_digest = "sha256:" + "d" * 64
        image_id = "sha256:" + "e" * 64
        self.ex.manifest = lambda ref: [{"Descriptor": {"digest": platform_digest, "platform": {"os": "linux", "architecture": "amd64"}}}]
        image = {"Id": image_id, "Os": "linux", "Architecture": "amd64", "RepoDigests": [self.ex.reference()],
                 "Config": {"Labels": {"org.opencontainers.image.revision": self.task["target_revision"]}}}
        with patch.object(executor, "command", side_effect=["pulled", json.dumps([image])]):
            result = self.ex.download()
        self.assertEqual(result["requested_digest"], requested)
        self.assertEqual(result["manifest_digest"], platform_digest)
        self.assertEqual(result["image_id"], image_id)

    def test_completion_intent_and_final_event_persist_together(self):
        self.claim()
        snapshots = []
        self.ex.save = lambda: snapshots.append(copy.deepcopy(self.ex.record))
        self.ex.phase("complete", "succeeded")
        self.assertTrue(snapshots)
        for snapshot in snapshots:
            self.assertTrue(snapshot["confirmed"] == "succeeded" or "succeeded" in snapshot["pending"])

    def test_compose_disables_old_restart_before_stop(self):
        self.claim()
        self.ex.cfg["mode"] = "compose"
        self.ex.cfg["service"] = "app"
        calls = []
        self.ex.compose = lambda *args: calls.append(["compose", *args])
        self.ex.inspect = lambda ref: {"State": {"Running": False}}
        with patch.object(executor, "command", side_effect=lambda args: calls.append(args) or ""):
            self.ex.stop_container()
        self.assertEqual(calls[0], ["docker", "update", "--restart=no", "old-container"])


if __name__ == "__main__":
    unittest.main()
