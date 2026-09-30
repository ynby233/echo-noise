#!/usr/bin/env python3
"""Single registered Linux Docker/Compose deployment; U4 must supply data protection."""
import argparse
import contextlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import time
from urllib.parse import urlsplit

IMAGE = "ghcr.io/ynby233/echo-noise"
VERSION = "u3-1"


class Stop(Exception):
    pass


def require(condition, code):
    if not condition:
        raise Stop(code)


def atomic_write(path, content):
    path = Path(path)
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix=".update-", dir=path.parent)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as output:
            output.write(content)
            output.flush()
            os.fsync(output.fileno())
        os.replace(name, path)
        if os.name == "posix":
            directory = os.open(path.parent, os.O_RDONLY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def write_image(path, reference):
    path = Path(path)
    lines = path.read_text().splitlines(keepends=True) if path.exists() else []
    require(sum(line.startswith("UPDATE_IMAGE=") for line in lines) <= 1, "duplicate_image_setting")
    found = False
    for i, line in enumerate(lines):
        if line.startswith("UPDATE_IMAGE="):
            lines[i] = "UPDATE_IMAGE=" + reference + "\n"
            found = True
    if not found:
        if lines and not lines[-1].endswith("\n"):
            lines[-1] += "\n"
        lines.append("UPDATE_IMAGE=" + reference + "\n")
    atomic_write(path, "".join(lines))


def private(path, directory=False):
    path = Path(path)
    require(path.exists() and not path.is_symlink(), "missing_or_symlink_control_file")
    if os.name == "posix":
        info = path.stat()
        require(info.st_uid in (0, os.geteuid()), "control_file_owner")
        require(not info.st_mode & (0o022 if directory else 0o077), "control_file_permissions")
        # A private file in a writable parent can still be replaced by another user.
        for parent in path.parents:
            require(not parent.stat().st_mode & 0o022 or parent.stat().st_mode & stat.S_ISVTX,
                    "control_parent_writable")


def command(args, *, input=None, timeout=300, env=None):
    try:
        result = subprocess.run(args, input=input, text=True, capture_output=True, timeout=timeout, env=env)
    except (OSError, subprocess.TimeoutExpired):
        raise Stop("command_unavailable_or_timeout") from None
    # Never forward Docker output: it can contain env/config values and private paths.
    require(result.returncode == 0, "command_failed:" + Path(args[0]).name)
    return result.stdout


def host_settings(config):
    result = {k: v for k, v in config.items() if k not in ("Binds", "Mounts")}
    # Engine startup normalizes an unset OOM flag to null; both keep OOM killing enabled.
    result["OomKillDisable"] = bool(result.get("OomKillDisable"))
    return result


class Executor:
    def __init__(self, config):
        self.config_path = Path(config).resolve()
        private(self.config_path)
        self.cfg = json.loads(self.config_path.read_text())
        required = {"url", "instance_id", "token_file", "state_dir", "backup_dir", "platform",
                    "mode", "mounts", "image_file"}
        allowed = required | {"container", "docker", "compose_file", "project", "service",
                              "min_free_bytes", "http_timeout", "health_timeout", "stop_timeout"}
        require(required <= self.cfg.keys() and self.cfg.keys() <= allowed, "config_fields")
        cfg = self.cfg
        url = urlsplit(cfg["url"])
        require(url.scheme == "https" or (url.scheme == "http" and url.hostname in ("localhost", "127.0.0.1", "::1")), "https_required")
        require(url.hostname and not url.username and not url.password and not url.query and not url.fragment, "url_invalid")
        require(re.fullmatch(r"[a-f0-9]{32}", cfg["instance_id"]), "instance_id_invalid")
        require(cfg["platform"] in ("linux/amd64", "linux/arm64"), "platform_unsupported")
        require(cfg["mode"] in ("docker", "compose"), "mode_invalid")
        for field in ("token_file", "state_dir", "backup_dir", "image_file"):
            require(Path(cfg[field]).is_absolute(), "absolute_control_path_required")
        private(cfg["token_file"])
        for field in ("state_dir", "backup_dir"):
            Path(cfg[field]).mkdir(mode=0o700, parents=True, exist_ok=True)
            private(cfg[field], directory=True)
        self.state = Path(cfg["state_dir"])
        self.journal = self.state / "active.json"
        self.record = None

    def api(self, method, path, body=None, token=None):
        token_path = Path(token or self.cfg["token_file"])
        private(token_path)
        raw = token_path.read_text().strip()
        require(re.fullmatch(r"enu_[a-f0-9]{64}", raw), "token_format")
        # The header travels only on stdin, never in argv, URL, trace or a saved log.
        args = ["curl", "--silent", "--connect-timeout", "10", "--max-time",
                str(self.cfg.get("http_timeout", 30)), "--proto", "=http,https",
                "--request", method, "--config", "-", "--write-out", "\n%{http_code}",
                self.cfg["url"].rstrip("/") + path]
        config = 'header = "Authorization: Bearer ' + raw + '"\nheader = "Content-Type: application/json"\n'
        if body is not None:
            config += "data = " + json.dumps(json.dumps(body)) + "\n"
        try:
            result = subprocess.run(args, input=config, text=True, capture_output=True,
                                    timeout=self.cfg.get("http_timeout", 30) + 5)
        except (OSError, subprocess.TimeoutExpired):
            raise Stop("http_transport") from None
        require(result.returncode == 0, "http_transport")
        payload, code = result.stdout.rsplit("\n", 1)
        require(code in ("200", "201", "204"), "http_" + code)
        if code == "204":
            return None
        value = json.loads(payload)
        require(value.get("code") == 1, "http_envelope")
        return value.get("data")

    def runtime(self):
        result = self.api("GET", "/api/updates/executor/runtime", token=self.record["token_file"] if self.record else None)
        require(result["instance_id"] == self.cfg["instance_id"], "instance_mismatch")
        return result

    def save(self):
        atomic_write(self.journal, json.dumps(self.record, indent=2) + "\n")

    def load_record(self):
        if not self.journal.exists():
            return
        private(self.journal)
        self.record = json.loads(self.journal.read_text())
        require(self.record["version"] == VERSION, "journal_version_unsupported")
        require(self.record["instance_id"] == self.cfg["instance_id"] and
                self.record["config_path"] == str(self.config_path) and
                self.record["deployment"] == self.deployment(), "journal_deployment_mismatch")

    def deployment(self):
        # Normal configuration equality, no hash or frozen protocol.
        return {k: v for k, v in self.cfg.items() if k not in ("token_file", "http_timeout", "health_timeout")}

    def claim(self):
        require(self.record is None, "local_record_exists")
        require(self.runtime()["instance_id"] == self.cfg["instance_id"], "instance_mismatch")
        old = self.preflight()
        task = self.api("POST", "/api/updates/executor/claim")
        if task is None:
            return
        require(re.fullmatch(r"[a-f0-9]{32}", task.get("id", "")) and
                task.get("target_image") == IMAGE and task.get("channel") in ("edge", "stable") and
                re.fullmatch(r"sha256:[a-f0-9]{64}", task.get("target_digest", "")) and
                re.fullmatch(r"[a-f0-9]{40}", task.get("target_revision", "")), "task_target_invalid")
        self.record = {"version": VERSION, "instance_id": self.cfg["instance_id"],
                       "config_path": str(self.config_path), "deployment": self.deployment(),
                       "token_file": self.cfg["token_file"], "task": task, "confirmed": task["status"],
                       "pending": [], "step": "claimed", "closed": False,
                       "old_container": old["Id"], "old_image": old["Image"],
                       "old_restart_policy": old.get("HostConfig", {}).get("RestartPolicy"),
                       "old_image_setting": Path(self.cfg["image_file"]).read_text() if Path(self.cfg["image_file"]).exists() else None,
                       "backup_path": str(Path(self.cfg["backup_dir"]) / task["id"])}
        if task["status"] != "claimed":
            self.record["step"] = "attention"
            self.record["error_code"] = "claimed_task_without_local_evidence"
        self.save()
        require(task["status"] == "claimed", "claimed_task_without_local_evidence")

    def queue(self, status):
        if (self.record["pending"] and self.record["pending"][-1] == status) or (not self.record["pending"] and self.record["confirmed"] == status):
            return
        self.record["pending"].append(status)
        self.save()

    def flush(self):
        while self.record["pending"]:
            status = self.record["pending"][0]
            self.api("POST", "/api/updates/executor/tasks/" + self.record["task"]["id"] + "/events",
                     {"status": status}, token=self.record["token_file"])
            self.record["confirmed"] = status
            self.record["pending"].pop(0)
            self.save()

    def phase(self, step, status=None):
        self.record["step"] = step
        if status:
            self.queue(status)
        self.save()

    def inspect(self, reference):
        return json.loads(command(["docker", "inspect", reference]))[0]

    def compose(self, *args, image_override=None):
        cfg = self.cfg
        environment = dict(os.environ)
        environment.pop("UPDATE_IMAGE", None)
        if image_override is not None:
            environment["UPDATE_IMAGE"] = image_override
        return command(["docker", "compose", "--project-name", cfg["project"], "--env-file", cfg["image_file"],
                        "--file", cfg["compose_file"], *args], env=environment)

    def check_compose_image_source(self):
        original = json.loads(self.compose("config", "--format", "json"))
        probe = IMAGE + "@sha256:" + "0" * 64
        changed = json.loads(self.compose("config", "--format", "json", image_override=probe))
        service = self.cfg["service"]
        require(changed["services"][service]["image"] == probe, "compose_target_image_not_variable")
        changed["services"][service]["image"] = original["services"][service]["image"]
        require(changed == original, "compose_image_variable_affects_other_settings")
        return original

    def container_id(self):
        if self.cfg["mode"] == "docker":
            return self.cfg["container"]
        ids = self.compose("ps", "--all", "--quiet", self.cfg["service"]).split()
        require(len(ids) == 1, "compose_requires_single_registered_container")
        return ids[0]

    def docker_create(self, name, image):
        opts = self.cfg.get("docker", {})
        allowed = {"network", "restart", "env_file", "devices", "ports", "log_driver", "log_options", "entrypoint", "command", "user"}
        require(opts.keys() <= allowed, "docker_option_unsupported")
        args = ["docker", "create", "--name", name, "--platform", self.cfg["platform"]]
        for key, flag in (("network", "--network"), ("restart", "--restart"), ("env_file", "--env-file"),
                          ("log_driver", "--log-driver"), ("entrypoint", "--entrypoint"), ("user", "--user")):
            if key in opts:
                args += [flag, str(opts[key])]
        for key, flag in (("devices", "--device"), ("ports", "--publish")):
            for item in opts.get(key, []):
                args += [flag, item]
        for key, value in opts.get("log_options", {}).items():
            args += ["--log-opt", key + "=" + value]
        for mount in self.cfg["mounts"]:
            args += ["--mount", "type=" + mount["type"] + ",source=" + mount["source"] + ",target=" + mount["target"] + (",readonly" if mount.get("read_only") else "")]
        args += [image, *opts.get("command", [])]
        return command(args).strip()

    def check_mounts(self, current):
        expected = sorted((m["type"], m["source"], m["target"], not m.get("read_only", False)) for m in self.cfg["mounts"])
        actual = sorted((m["Type"], m.get("Name") if m["Type"] == "volume" else m["Source"], m["Destination"], m["RW"]) for m in current["Mounts"])
        require(actual == expected, "registered_mounts_mismatch")
        controls = [self.config_path, Path(__file__).resolve(), self.state, Path(self.cfg["backup_dir"]),
                    Path(self.cfg["token_file"]), Path(self.cfg["image_file"])]
        if self.cfg["mode"] == "compose":
            controls.append(Path(self.cfg["compose_file"]))
        elif self.cfg.get("docker", {}).get("env_file"):
            controls.append(Path(self.cfg["docker"]["env_file"]))
        for mount in current["Mounts"]:
            source = Path(mount["Source"]).resolve()
            require(all(not p.resolve().is_relative_to(source) for p in controls), "app_can_access_executor_controls")
            require(mount["Destination"] != "/var/run/docker.sock", "docker_socket_in_application")

    def preflight(self):
        require(shutil.which("docker") and shutil.which("curl"), "install_docker_cli_and_curl")
        require(command(["docker", "info", "--format", "{{.OSType}}/{{.Architecture}}"] ).strip() in
                (self.cfg["platform"], self.cfg["platform"].replace("amd64", "x86_64").replace("arm64", "aarch64")), "host_platform_mismatch")
        private(self.cfg["image_file"])
        if self.cfg["mode"] == "compose":
            private(self.cfg["compose_file"])
            model = self.check_compose_image_source()
            service = model["services"][self.cfg["service"]]
            require("build" not in service and "image" in service, "compose_prebuilt_image_required")
            require(Path(self.cfg["compose_file"]).read_text().count("${UPDATE_IMAGE") == 1, "compose_single_image_setting_required")
        elif self.cfg.get("docker", {}).get("env_file"):
            private(self.cfg["docker"]["env_file"])
        current = self.inspect(self.container_id())
        require(current["State"]["Running"], "registered_container_not_running")
        image = json.loads(command(["docker", "image", "inspect", current["Image"]]))[0]
        require(image["Os"] + "/" + image["Architecture"] == self.cfg["platform"], "installed_platform_mismatch")
        self.check_mounts(current)
        if self.cfg["mode"] == "docker":
            probe = self.cfg["container"] + "-update-check"
            probe_id = self.docker_create(probe, current["Image"])
            try:
                configured = self.inspect(probe_id)
                # -v and --mount encode the same mount differently in HostConfig.
                self.check_mounts(configured)
                for m in current["Mounts"]:
                    require(m.get("Propagation", "") in ("", "rprivate") and m.get("Driver", "local") == "local", "mount_option_unsupported")
                actual, desired = host_settings(current["HostConfig"]), host_settings(configured["HostConfig"])
                differences = sorted(k for k in actual.keys() | desired.keys() if actual.get(k) != desired.get(k))
                require(not differences, "docker_parameters_not_represented:" + ",".join(differences))
                for key in ("Env", "Cmd", "Entrypoint", "User", "WorkingDir", "Healthcheck", "Labels", "Volumes", "ExposedPorts", "StopSignal"):
                    require(current["Config"].get(key) == configured["Config"].get(key), "docker_config_not_represented")
            finally:
                command(["docker", "rm", probe_id])
        self.check_space()
        docker_root = command(["docker", "info", "--format", "{{.DockerRootDir}}"] ).strip()
        require(Path(docker_root).is_dir(), "local_docker_engine_required")
        require(shutil.disk_usage(docker_root).free >= self.cfg.get("min_free_bytes", 1024**3), "docker_disk_space")
        return current

    def check_space(self):
        for field in ("state_dir", "backup_dir"):
            require(shutil.disk_usage(self.cfg[field]).free >= self.cfg.get("min_free_bytes", 1024**3), "disk_space")

    def reference(self):
        return IMAGE + "@" + self.record["task"]["target_digest"]

    def manifest(self, reference):
        return json.loads(command(["docker", "manifest", "inspect", "--verbose", reference]))

    def download(self):
        reference = self.reference()
        manifest = self.manifest(reference)
        descriptors = manifest if isinstance(manifest, list) else [manifest]
        matches = [m["Descriptor"] for m in descriptors if
                   m.get("Descriptor", {}).get("platform", {}).get("os") == "linux" and
                   m["Descriptor"]["platform"].get("architecture") == self.cfg["platform"].split("/")[1]]
        require(len(matches) == 1, "registry_platform_unavailable")
        command(["docker", "pull", "--platform", self.cfg["platform"], reference], timeout=1800)
        image = json.loads(command(["docker", "image", "inspect", reference]))[0]
        require(image["Os"] + "/" + image["Architecture"] == self.cfg["platform"], "pulled_platform_mismatch")
        require(reference in image.get("RepoDigests", []), "pulled_digest_mismatch")
        require(image["Config"].get("Labels", {}).get("org.opencontainers.image.revision") == self.record["task"]["target_revision"], "pulled_revision_mismatch")
        return {"requested_digest": self.record["task"]["target_digest"], "manifest_digest": matches[0]["digest"],
                "image_id": image["Id"], "repo_digests": image["RepoDigests"]}

    def data_protection_available(self):
        return False  # U4: validated SQLite/layout backup and controlled shutdown capability.

    def backup(self):
        raise Stop("u4_backup_unavailable")

    def stop_container(self):
        command(["docker", "update", "--restart=no", self.record["old_container"]])
        if self.cfg["mode"] == "compose":
            self.compose("stop", "--timeout", str(self.cfg.get("stop_timeout", 60)), self.cfg["service"])
        else:
            command(["docker", "stop", "--time", str(self.cfg.get("stop_timeout", 60)), self.record["old_container"]])
        require(not self.inspect(self.record["old_container"])["State"]["Running"], "old_writer_still_running")

    def replace(self):
        write_image(self.cfg["image_file"], self.reference())
        if self.cfg["mode"] == "compose":
            service = json.loads(self.compose("config", "--format", "json"))["services"][self.cfg["service"]]
            require(service["image"] == self.reference(), "compose_target_image_mismatch")
            self.compose("up", "--detach", "--no-deps", "--no-build", "--pull", "never", self.cfg["service"])
        else:
            command(["docker", "rename", self.record["old_container"], self.cfg["container"] + "-previous-" + self.record["task"]["id"]])
            new_id = self.docker_create(self.cfg["container"], self.record["download"]["image_id"])
            command(["docker", "start", new_id])

    def target_running(self):
        try:
            current = self.inspect(self.container_id())
            return current["Image"] == self.record["download"]["image_id"] and current["State"]["Running"]
        except Stop:
            return False

    def verify(self):
        deadline = time.monotonic() + self.cfg.get("health_timeout", 120)
        while time.monotonic() < deadline:
            current = self.inspect(self.container_id())
            require(current["Image"] == self.record["download"]["image_id"], "running_image_mismatch")
            self.check_mounts(current)
            if current["State"].get("Health", {}).get("Status") == "healthy":
                try:
                    runtime = self.runtime()
                    require(runtime["revision"] == self.record["task"]["target_revision"], "runtime_revision_mismatch")
                    return
                except Stop as error:
                    if str(error) != "http_transport":
                        raise
            time.sleep(2)
        raise Stop("target_health_timeout")

    def attention(self, code):
        self.phase("attention")
        self.record["error_code"] = code
        self.save()
        self.queue("needs_attention")
        with contextlib.suppress(Stop):
            self.flush()
        raise Stop(code)

    def run(self):
        if self.record is None:
            self.load_record()
        if self.record is None:
            self.claim()
        if self.record is None or self.record["closed"]:
            return
        # Terminal retries use the pinned credential and no claim/runtime operation.
        self.flush()
        if self.record["step"] == "complete":
            require(self.record["confirmed"] == "succeeded", "journal_terminal_unconfirmed")
            self.record["closed"] = True
            self.save()
            return
        step = self.record["step"]
        if step == "attention":
            raise Stop("manual_reconciliation_required")
        if step == "replace_intent":
            require("download" in self.record, "journal_missing_image")
            if not self.target_running():
                self.attention("replacement_outcome_unknown")
            self.phase("replaced")
        elif step in ("stop_intent", "stopped", "backup_intent", "backed_up"):
            self.attention("interrupted_destructive_step")
        elif step in ("claimed", "download_intent", "downloaded"):
            self.runtime()
            current = self.preflight()
            require(current["Id"] == self.record["old_container"] and current["Image"] == self.record["old_image"], "registered_container_changed")
            if step != "downloaded":
                self.phase("download_intent", "downloading")
                self.flush()  # Before downtime, auth/report errors prevent host mutation.
                self.record["download"] = self.download()
                self.phase("downloaded")
            require(self.data_protection_available(), "u4_backup_unavailable")
            self.check_space()
            self.phase("stop_intent", "stopping")
            self.flush()
            try:
                self.stop_container()
                self.phase("stopped")
                self.phase("backup_intent", "backing_up")
                self.backup()
                self.phase("backed_up")
                self.phase("replace_intent", "replacing")
                self.replace()
                self.phase("replaced")
            except Stop as error:
                self.attention(str(error))
        try:
            self.phase("verifying", "verifying")
            self.verify()
        except Stop as error:
            self.attention(str(error))
        self.phase("complete", "succeeded")
        self.flush()
        self.record["closed"] = True
        self.save()

    def archive_closed(self):
        if self.record and self.record["closed"]:
            os.replace(self.journal, self.state / (self.record["task"]["id"] + ".json"))
            self.record = None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("check", "claim", "report", "run"))
    parser.add_argument("config")
    args = parser.parse_args()
    try:
        require(sys.platform == "linux", "linux_host_required")
        import fcntl
        ex = Executor(args.config)
        with open(ex.state / "executor.lock", "a") as lock:
            os.chmod(lock.name, 0o600)
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                raise Stop("executor_already_running") from None
            ex.load_record()
            if args.action == "check":
                ex.runtime()
                ex.preflight()
                print(VERSION + ": deployment checked; installation requires U4 backup")
            elif args.action == "report":
                require(ex.record is not None, "no_local_record")
                ex.flush()
            else:
                ex.archive_closed()
                if args.action == "claim":
                    if ex.record is None:
                        ex.claim()
                else:
                    ex.run()
                print("idle" if ex.record is None else "task=" + ex.record["task"]["id"] + " step=" + ex.record["step"])
    except (Stop, ValueError, KeyError, OSError) as error:
        # Only finite error codes leave the process; parser/filesystem messages may expose secrets.
        print("executor: " + (str(error) if isinstance(error, Stop) else "invalid_config_or_record"), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
