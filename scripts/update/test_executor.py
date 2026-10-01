"""Run with python3 -m unittest discover -s scripts/update -p 'test_*.py'."""
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import types
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
        self.ex.inspect = lambda _: {"Config": {}, "Mounts": [], "State": {"Running": True}, "Id": "old-container"}
        self.ex.check_writers = lambda *a, **k: None
        self.ex.offline_tool = lambda *a, **k: (_ for _ in ()).throw(executor.Stop("old_tool_missing"))
        self.ex.stop_container = lambda: self.fail("stopped without U4")
        with self.assertRaisesRegex(executor.Stop, "u4_backup_unavailable"):
            self.ex.run()

    def test_backup_failure_restarts_only_unchanged_old_writer(self):
        self.claim()
        self.ex.download = lambda: {"image_id": "new"}
        self.ex.data_protection_available = lambda: True
        self.ex.prepare_shutdown = lambda: None
        self.ex.stop_container = lambda: None
        self.ex.backup = lambda: (_ for _ in ()).throw(executor.Stop("backup_failed"))
        restarted = []
        self.ex.restore_old = lambda: restarted.append(True)
        self.ex.replace = lambda: self.fail("replaced after failed backup")
        with self.assertRaisesRegex(executor.Stop, "backup_failed"):
            self.ex.run()
        self.assertEqual(restarted, [True])
        self.assertTrue(self.ex.record["closed"])
        self.assertEqual(self.ex.record["confirmed"], "failed")

    def test_failed_terminal_retry_needs_no_runtime(self):
        self.claim()
        self.ex.phase("failed", "failed")
        self.ex.runtime = lambda: self.fail("failed retry called runtime")
        self.ex.run()
        self.assertTrue(self.ex.record["closed"])

    def test_forced_shutdown_cannot_enter_backup(self):
        self.claim()
        self.ex.inspect = lambda _: {"State": {"Running": False, "ExitCode": 137, "OOMKilled": False}, "HostConfig": {"RestartPolicy": {"Name": "no"}}}
        with patch.object(executor, "command", return_value=""):
            with self.assertRaisesRegex(executor.Stop, "old_shutdown_not_clean"):
                self.ex.stop_container()

    def test_lost_prepare_and_cancel_preserves_attention(self):
        self.claim()
        self.ex.download = lambda: {"image_id": "new"}
        self.ex.data_protection_available = lambda: True
        self.ex.prepare_shutdown = lambda: (_ for _ in ()).throw(executor.Stop("http_transport"))
        normal = self.api
        def api(method, path, body=None, token=None):
            if path.endswith("prepare"):
                raise executor.Stop("http_transport")
            return normal(method, path, body, token)
        self.ex.api = api
        self.ex.stop_container = lambda: self.fail("stopped after unconfirmed prepare")
        with self.assertRaisesRegex(executor.Stop, "shutdown_preparation_requires_reconciliation"):
            self.ex.run()
        self.assertEqual(self.ex.record["step"], "attention")
        self.assertEqual(self.ex.record["confirmed"], "needs_attention")
        self.assertFalse(self.ex.record["closed"])

    def test_started_target_failure_never_restarts_old(self):
        self.claim()
        self.ex.record.update(step="replace_intent", download={"image_id": "new"})
        self.ex.target_running = lambda: True
        self.ex.verify = lambda: (_ for _ in ()).throw(executor.Stop("target_health_timeout"))
        self.ex.restore_old = lambda: self.fail("old restarted after new writes")
        with self.assertRaisesRegex(executor.Stop, "target_health_timeout"):
            self.ex.run()
        self.assertEqual(self.ex.record["confirmed"], "needs_attention")

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
        self.ex.inspect = lambda ref: {"State": {"Running": False, "ExitCode": 0}, "HostConfig": {"RestartPolicy": {"Name": "no"}}}
        with patch.object(executor, "command", side_effect=lambda args: calls.append(args) or ""):
            self.ex.stop_container()
        self.assertEqual(calls[0], ["docker", "update", "--restart=no", "old-container"])

    def test_compose_variable_must_select_only_registered_service(self):
        original = {"services": {"app": {"image": "old"}, "other": {"image": "old"}}}
        changed = {"services": {"app": {"image": "old"}, "other": {"image": "target"}}}
        self.ex.cfg["service"] = "app"
        self.ex.compose = lambda *a, **k: json.dumps(changed if k else original)
        with self.assertRaises(executor.Stop):
            self.ex.check_compose_image_source()

    def test_engine_normalized_oom_flag_keeps_true_distinct(self):
        self.assertEqual(executor.host_settings({"OomKillDisable": None}), executor.host_settings({"OomKillDisable": False}))
        self.assertNotEqual(executor.host_settings({"OomKillDisable": True}), executor.host_settings({"OomKillDisable": False}))

    def test_attention_first_write_retains_event_and_error_on_restart(self):
        self.claim()
        self.ex.record.update(step="stop_intent", confirmed="stopping")
        self.ex.save()
        save = self.ex.save
        def killed():
            save()
            raise KeyboardInterrupt()
        with patch.object(self.ex, "save", side_effect=killed):
            with self.assertRaises(KeyboardInterrupt):
                self.ex.attention("interrupted_destructive_step")
        self.ex.load_record()
        self.assertEqual(self.ex.record.get("error_code"), "interrupted_destructive_step")
        self.assertEqual(self.ex.record["pending"], ["needs_attention"])
        with self.assertRaisesRegex(executor.Stop, "manual_reconciliation_required"):
            self.ex.run()
        self.assertEqual(self.ex.record["confirmed"], "needs_attention")

    def test_legacy_attention_without_event_is_reported_without_host_mutation(self):
        self.claim()
        self.ex.record.update(step="attention", confirmed="stopping", pending=[])
        self.ex.save()
        self.ex.stop_container = self.ex.replace = lambda: self.fail("repeated host mutation")
        with self.assertRaisesRegex(executor.Stop, "manual_reconciliation_required"):
            self.ex.run()
        self.assertEqual(self.ex.record["confirmed"], "needs_attention")

    def test_attention_save_failure_before_and_after_commit_recovers_without_replacement(self):
        self.claim()
        save = self.ex.save
        for after in (False, True):
            with self.subTest(after=after):
                self.ex.record.update(step="stop_intent", confirmed="stopping", pending=[])
                save()
                def failed():
                    if after: save()
                    raise OSError("injected journal failure")
                with patch.object(self.ex, "save", side_effect=failed):
                    with self.assertRaises(OSError):
                        self.ex.attention("interrupted_destructive_step")
                self.ex.load_record()
                self.ex.stop_container = self.ex.replace = lambda: self.fail("repeated destructive operation")
                with self.assertRaises(executor.Stop): self.ex.run()
                self.assertEqual(self.ex.record["confirmed"], "needs_attention")
                self.assertEqual(self.ex.record["task"]["id"], self.task["id"])

    def test_attention_http_failures_retain_event_and_legacy_claimed_needs_reconciliation(self):
        self.claim()
        for error in ("http_401", "http_409", "http_transport"):
            with self.subTest(error=error):
                self.ex.record.update(step="attention", confirmed="stopping", pending=[])
                self.ex.save()
                self.ex.api = lambda *a, **k: (_ for _ in ()).throw(executor.Stop(error))
                with self.assertRaisesRegex(executor.Stop, error): self.ex.run()
                self.ex.load_record()
                self.assertEqual(self.ex.record["pending"], ["needs_attention"])
                self.assertFalse(self.ex.record["closed"])
        self.ex.record.update(confirmed="claimed", pending=[])
        self.ex.save()
        with self.assertRaisesRegex(executor.Stop, "attention_evidence_requires_reconciliation"):
            self.ex.flush()
        self.assertEqual(self.ex.record["confirmed"], "claimed")

    def test_untrusted_parent_owner_and_symlink_are_rejected(self):
        class Metadata:
            def __init__(self, uid, mode, parents=(), symlink=False):
                self.parents, self.symlink = parents, symlink
                self.info = types.SimpleNamespace(st_uid=uid, st_mode=mode)
            def exists(self): return True
            def is_symlink(self): return self.symlink
            def stat(self): return self.info
        for parent in (Metadata(1001, 0o40700), Metadata(1001, 0o41777),
                       Metadata(0, 0o40755, symlink=True), Metadata(0, 0o40777)):
            with self.subTest(info=parent.info, symlink=parent.symlink), \
                    patch.object(executor, "Path", lambda p: p), \
                    patch.object(executor.os, "name", "posix"), \
                    patch.object(executor.os, "geteuid", return_value=0, create=True):
                with self.assertRaises(executor.Stop):
                    executor.private(Metadata(0, 0o100600, [parent]))
        for uid, mode in ((0, 0o40755), (0, 0o41777), (1000, 0o40700)):
            with patch.object(executor, "Path", lambda p: p), \
                    patch.object(executor.os, "name", "posix"), \
                    patch.object(executor.os, "geteuid", return_value=1000, create=True):
                executor.private(Metadata(1000, 0o100600, [Metadata(uid, mode)]))

    def test_socket_source_alias_and_directory_are_rejected(self):
        directory = self.root / "engine"
        directory.mkdir()
        endpoint = directory / "docker.sock"
        endpoint.touch()
        for source in (endpoint, directory):
            self.ex.cfg["mounts"] = [{"type": "bind", "source": str(source), "target": "/host/engine"}]
            with self.subTest(source=source), \
                    patch.dict(executor.os.environ, {"DOCKER_HOST": "unix://" + str(endpoint)}, clear=True), \
                    patch.object(executor, "command", return_value=json.dumps("unix://" + str(endpoint))), \
                    patch.object(executor.stat, "S_ISSOCK", return_value=True):
                with self.assertRaisesRegex(executor.Stop, "docker_socket_in_application"):
                    self.ex.check_mounts({"Mounts": [{"Type": "bind", "Source": str(source),
                        "Destination": "/host/engine", "RW": True}]})

    def test_normal_bind_and_named_volume_do_not_expose_socket(self):
        endpoint = self.root / "docker.sock"
        endpoint.touch()
        source = self.root / "application"
        source.mkdir()
        for kind in ("bind", "volume"):
            self.ex.cfg["mounts"] = [{"type": kind, "source": str(source) if kind == "bind" else "data", "target": "/data"}]
            with patch.dict(executor.os.environ, {"DOCKER_HOST": "unix://" + str(endpoint)}, clear=True), \
                    patch.object(executor.stat, "S_ISSOCK", return_value=True), \
                    patch.object(executor, "command", return_value=json.dumps([
                        {"Name": "data", "Driver": "local", "Options": None}])):
                self.ex.check_mounts({"Mounts": [{"Type": kind, "Name": "data", "Source": str(source),
                                                "Destination": "/data", "RW": True}]})


class PreflightTests(unittest.TestCase):
    api = RecoveryTests.api

    def setUp(self):
        RecoveryTests.setUp(self)
        del self.ex.preflight
        Path(self.cfg["image_file"]).write_text("UPDATE_IMAGE=old\n")
        Path(self.cfg["image_file"]).chmod(0o600)
        self.current = {"Id": "c" * 64, "Image": "old-image", "State": {"Running": True}, "Mounts": [],
                        "HostConfig": {"NetworkMode": "appnet"},
                        "Config": {"Hostname": "c" * 12, "Domainname": "", "Labels": {}},
                        "NetworkSettings": {"Networks": {"appnet": {}}}}
        self.probe = copy.deepcopy(self.current)
        self.probe.update(Id="d" * 64, State={"Running": False, "Status": "created"})
        self.probe["Config"]["Hostname"] = "d" * 12
        self.probe_present = False
        self.create_loss = False
        self.commands = []
        self.ex.inspect = lambda ref: self.probe if ref in ("d" * 64, self.cfg["container"] + "-update-check") else self.current
        self.addCleanup(patch.stopall)
        (self.root / "socket").touch()
        patch.dict(executor.os.environ, {"DOCKER_HOST": "", "DOCKER_CONTEXT": ""}).start()
        patch.object(executor.stat, "S_ISSOCK", return_value=True).start()
        patch.object(executor, "command", side_effect=self.command).start()
        patch.object(executor.shutil, "which", return_value="present").start()

    def command(self, args, **kwargs):
        self.commands.append(args)
        if "{{.OSType}}/{{.Architecture}}" in args: return "linux/amd64"
        if "{{.DockerRootDir}}" in args: return str(self.root)
        if args[1:3] == ["context", "inspect"]: return json.dumps("unix://" + str(self.root / "socket"))
        if args[1:3] == ["image", "inspect"]: return json.dumps([{"Os": "linux", "Architecture": "amd64"}])
        if args[1:3] == ["container", "ls"]: return "d" * 64 if self.probe_present else ""
        if args[1] == "create":
            if self.probe_present: raise executor.Stop("command_failed:docker")
            self.probe_present = True
            labels = {}
            for i, arg in enumerate(args):
                if arg == "--label":
                    key, value = args[i + 1].split("=", 1)
                    labels[key] = value
            self.probe["Config"]["Labels"] = labels
            self.probe["Name"] = "/" + self.cfg["container"] + "-update-check"
            if self.create_loss: raise executor.Stop("command_unavailable_or_timeout")
            return "d" * 64
        if args[1] == "rm": self.probe_present = False; return ""
        raise AssertionError(args)

    def test_extra_network_hostname_domain_and_endpoint_customization_rejected(self):
        changes = (("extra", lambda: self.current["NetworkSettings"]["Networks"].update(dbnet={})),
                   ("hostname", lambda: self.current["Config"].update(Hostname="custom")),
                   ("domain", lambda: self.current["Config"].update(Domainname="custom.test")),
                   ("endpoint", lambda: self.current["NetworkSettings"]["Networks"]["appnet"].update(IPAMConfig={"IPv4Address": "172.20.0.9"})))
        original = copy.deepcopy(self.current)
        for name, change in changes:
            with self.subTest(name=name):
                self.current = copy.deepcopy(original)
                change()
                with self.assertRaises(executor.Stop): self.ex.preflight()
                self.assertFalse(self.probe_present)
        self.assertFalse(any(a[1] in ("stop", "rename", "start") for a in self.commands))

    def test_compose_model_multiple_replicas_with_single_current_container_rejected(self):
        path = self.root / "compose.json"
        self.ex.cfg.update(mode="compose", service="app", project="isolated", compose_file=str(path))
        for settings in ({"scale": 2}, {"deploy": {"replicas": 2}}, {"deploy": {"mode": "global"}}):
            with self.subTest(settings=settings):
                model = {"services": {"app": {"image": "${UPDATE_IMAGE}", **settings}}}
                path.write_text(json.dumps(model)); path.chmod(0o600)
                def compose(*args, image_override=None):
                    if args[0] == "ps": return self.current["Id"]
                    parsed = copy.deepcopy(model)
                    parsed["services"]["app"]["image"] = image_override or "old"
                    return json.dumps(parsed)
                self.ex.compose = compose
                with self.assertRaisesRegex(executor.Stop, "compose_requires_single_replica"):
                    self.ex.preflight()

    def test_create_response_loss_is_cleaned_and_preflight_retry_succeeds(self):
        self.create_loss = True
        with self.assertRaisesRegex(executor.Stop, "command_unavailable_or_timeout"):
            self.ex.preflight()
        self.assertFalse(self.probe_present)
        self.create_loss = False
        self.assertEqual(self.ex.preflight()["Id"], self.current["Id"])
        self.assertFalse(self.probe_present)

    def test_no_mounts_remote_engine_is_rejected_before_deployment_operations(self):
        for environment in ({"DOCKER_HOST": "tcp://remote-host:2375"},
                            {"DOCKER_CONTEXT": "remote", "DOCKER_HOST": "unix:///ignored.sock"}):
            with self.subTest(environment=environment), patch.dict(executor.os.environ, environment, clear=True):
                before = len(self.commands)
                command = self.command
                def remote_context(args, **kwargs):
                    if args[1:3] == ["context", "inspect"]:
                        self.commands.append(args)
                        # Context metadata must explicitly name the selected context.
                        return json.dumps("tcp://remote-host:2375" if "remote" in args else
                                          "unix://" + str(self.root / "socket"))
                    return command(args, **kwargs)
                with patch.object(executor, "command", side_effect=remote_context):
                    with self.assertRaisesRegex(executor.Stop, "local_unix_docker_endpoint_required"):
                        self.ex.preflight()
                self.assertFalse(self.probe_present)
                self.assertFalse(any(a[1] in ("info", "create", "rm", "inspect")
                                     for a in self.commands[before:]))

    def test_named_volume_options_are_rejected_in_both_deployment_modes(self):
        source = self.root / "volume-data"
        source.mkdir()
        mount = {"Type": "volume", "Name": "app-data", "Source": str(source),
                 "Destination": "/data", "RW": True, "Driver": "local"}
        self.current["Mounts"] = [mount]
        self.probe["Mounts"] = [copy.deepcopy(mount)]
        self.ex.cfg["mounts"] = [{"type": "volume", "source": "app-data", "target": "/data"}]
        compose_file = self.root / "compose.json"
        compose_file.write_text('${UPDATE_IMAGE}')
        compose_file.chmod(0o600)
        self.ex.cfg.update(service="app", compose_file=str(compose_file))
        self.ex.check_compose_image_source = lambda: {"services": {"app": {"image": "old"}}}
        self.ex.compose = lambda *a, **k: self.current["Id"]
        for mode in ("docker", "compose"):
            for options in ({"type": "none", "o": "bind", "device": "/run"},
                            {"type": "nfs", "device": ":/data"}):
                with self.subTest(mode=mode, options=options):
                    self.ex.cfg["mode"] = mode
                    command = self.command
                    def volume_info(args, **kwargs):
                        if args[1:3] == ["volume", "inspect"]:
                            return json.dumps([{"Name": "app-data", "Driver": "local", "Options": options}])
                        return command(args, **kwargs)
                    before = len(self.commands)
                    with patch.object(executor, "command", side_effect=volume_info):
                        with self.assertRaisesRegex(executor.Stop, "named_volume_options_unsupported"):
                            self.ex.preflight()
                    self.assertFalse(any(a[1] in ("create", "stop", "rename", "start")
                                         for a in self.commands[before:]))

    def test_foreign_and_running_probe_are_never_removed(self):
        for own, running in ((False, False), (True, True)):
            with self.subTest(own=own, running=running):
                self.probe_present = True
                self.probe["Name"] = "/" + self.cfg["container"] + "-update-check"
                self.probe["Config"]["Labels"] = self.ex.probe_labels() if own else {}
                self.probe["State"] = {"Running": running, "Status": "running" if running else "created"}
                before = len(self.commands)
                with self.assertRaisesRegex(executor.Stop, "probe_"): self.ex.preflight()
                self.assertFalse(any(a[1] == "rm" for a in self.commands[before:]))
                self.assertTrue(self.probe_present)

    def test_explicit_mac_address_is_rejected_without_stopping_current_container(self):
        self.current["Config"]["MacAddress"] = "02:42:ac:11:00:77"
        with self.assertRaisesRegex(executor.Stop, "docker_mac_address_not_represented"):
            self.ex.preflight()
        self.assertFalse(self.probe_present)
        self.assertTrue(self.current["State"]["Running"])
        self.assertFalse(any(a[1] in ("stop", "rename", "start") for a in self.commands))

    def test_default_and_host_network_pass_and_clean_probe(self):
        for network in ("appnet", "host"):
            self.current["HostConfig"]["NetworkMode"] = self.probe["HostConfig"]["NetworkMode"] = network
            self.current["Config"]["MacAddress"] = ""
            self.probe["Config"]["MacAddress"] = None
            self.current["NetworkSettings"]["Networks"] = {network: {"IPAddress": "dynamic", "EndpointID": "old", "MacAddress": "dynamic"}}
            self.probe["NetworkSettings"]["Networks"] = {network: {"IPAddress": "", "EndpointID": "", "MacAddress": ""}}
            self.assertEqual(self.ex.preflight()["Id"], self.current["Id"])
            self.assertFalse(self.probe_present)

    def test_host_network_inherited_engine_hostname_is_preserved(self):
        self.current["HostConfig"]["NetworkMode"] = self.probe["HostConfig"]["NetworkMode"] = "host"
        self.current["NetworkSettings"]["Networks"] = self.probe["NetworkSettings"]["Networks"] = {"host": {}}
        self.current["Config"]["Hostname"] = self.probe["Config"]["Hostname"] = "engine-host"
        self.assertEqual(self.ex.preflight()["Id"], self.current["Id"])

    def test_compose_default_and_explicit_one_pass(self):
        path = self.root / "compose.json"
        path.write_text('${UPDATE_IMAGE}'); path.chmod(0o600)
        self.ex.cfg.update(mode="compose", service="app", compose_file=str(path))
        for settings in ({}, {"scale": 1}, {"deploy": {"replicas": 1, "mode": "replicated"}}):
            self.ex.check_compose_image_source = lambda: {"services": {"app": {"image": "old", **settings}}}
            self.ex.compose = lambda *a, **k: self.current["Id"]
            self.assertEqual(self.ex.preflight()["Id"], self.current["Id"])


if __name__ == "__main__":
    unittest.main()
