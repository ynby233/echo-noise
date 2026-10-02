#!/usr/bin/env python3
"""Real Dagu/container test, run only in an independent Linux Docker Engine."""
import concurrent.futures
import contextlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import sqlite3
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
from unittest import mock
import zipfile

import executor

spec = importlib.util.spec_from_file_location("engine_fixture", Path(__file__).with_name("test-docker.py"))
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)
fixture.unittest_patch = mock.patch
docker = fixture.docker


def http(url, method="GET", body=None, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(url, method=method, headers=headers,
                                     data=json.dumps(body).encode() if body is not None else None)
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            data = response.read()
            return response.status, json.loads(data) if data else None
    except urllib.error.HTTPError as error:
        try:
            return error.code, json.loads(error.read())
        except ValueError:
            return error.code, None


def eventually(check, timeout=100):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            if check():
                return
        except (OSError, ValueError):
            pass
        time.sleep(1)
    raise AssertionError("Dagu test condition timed out")


def readiness():
    return http("http://127.0.0.1:1314/fixture/state")[1]["data"]["installation"]["available"]


def test_case(root, refs, scenario, restart_second=None):
    case = root / scenario
    case.mkdir(mode=0o700)
    control, data, config, home = [case / name for name in ("control", "data", "config", "dagu")]
    for directory in (control, data, config, home):
        directory.mkdir(mode=0o700)
    (config / "config.yaml").write_text("database:\n  type: sqlite\n  path: /data/fixture.db\n")
    (config / "runtime.env").write_text("DB_PATH=/data/fixture.db\n")
    (data / "sentinel").write_bytes(b"preserved attachment")
    if scenario == "wake":
        # Real NAS regression: user-owned private files must be readable by the
        # offline helper while its business mounts still forbid writes.
        for path in (config / "runtime.env", data / "sentinel"):
            os.chown(path, 1000, 1000)
            path.chmod(0o600)
        docker("run", "--rm", "--cap-drop", "ALL", "--cap-add", "DAC_OVERRIDE",
               "--mount", "type=bind,source=" + str(config) + ",target=/app/config,readonly",
               "--entrypoint", "/bin/sh", refs[0][0], "-c",
               "test -r /app/config/runtime.env && ! echo mutation > /app/config/runtime.env")
        assert (config / "runtime.env").read_text() == "DB_PATH=/data/fixture.db\n"
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
    dagu_url = "http://127.0.0.1:" + str(port)
    task_name = "echo-noise-update"
    wake_url = dagu_url + "/api/v1/webhooks/" + task_name
    executor.atomic_write(config / "wake-token", "placeholder")
    executor.atomic_write(control / "app.env", "FIXTURE_TARGET_DIGEST=" + refs[1][1] +
                          "\nFIXTURE_TARGET_REVISION=" + "2" * 40 +
                          "\nUPDATE_EXECUTOR_WAKE_URL=" + ("http://127.0.0.1:9/missed" if scenario == "missed" else wake_url) +
                          "\nUPDATE_EXECUTOR_WAKE_TOKEN_FILE=/app/config/wake-token\n")
    app = fixture.prefix + "-" + scenario
    mounts = [{"type": "bind", "source": str(data), "target": "/data"},
              {"type": "bind", "source": str(data), "target": "/app/data"},
              {"type": "bind", "source": str(config), "target": "/app/config"}]
    args = ["run", "-d", "--name", app, "--network", "host", "--restart=no", "--env-file", str(control / "app.env")]
    if scenario == "wake":
        args += ["--hostname", "registered-app-host"]
    for mount in mounts:
        args += ["--mount", "type=bind,source=" + mount["source"] + ",target=" + mount["target"]]
    old = docker(*args, refs[0][0])
    eventually(lambda: http("http://127.0.0.1:1314/health")[0] == 200)
    shutil.copyfile(data / "token", control / "token")
    (control / "token").chmod(0o600)
    executor.atomic_write(control / "image.env", "UPDATE_IMAGE=" + refs[0][0] + "\n")
    cfg = {"url": "http://127.0.0.1:1314", "instance_id": (data / "instance").read_text(),
           "token_file": str(control / "token"), "state_dir": str(control / "state"),
           "backup_dir": str(control / "backups"), "image_file": str(control / "image.env"),
           "platform": "linux/amd64", "mode": "docker", "container": app, "mounts": mounts,
           "docker": {"network": "host", "restart": "no", "env_file": str(control / "app.env")},
           "min_free_bytes": 1024, "health_timeout": 5}
    if scenario == "wake":
        cfg["docker"]["hostname"] = "registered-app-host"
    executor.atomic_write(control / "executor.json", json.dumps(cfg))
    (home / "dags").mkdir()
    script_root = Path("scripts/update").resolve()
    definition = ("schedule: '* * * * *'\nmax_active_runs: 1\nenv:\n  FIXTURE_REGISTRY: " + json.dumps(fixture.registry_image) +
                  "\n  FIXTURE_HOLD_AFTER_REPLACE: " + json.dumps("1" if scenario == "restart" else "0") +
                  "\nsteps:\n  - id: executor\n    run: python3 " + str(script_root / "fixture/container-executor.py") + " run " + str(control / "executor.json") + "\n")
    (home / "dags" / (task_name + ".yaml")).write_text(definition)
    scheduler = fixture.prefix + "-dagu-" + scenario
    scheduler_args = ["run", "-d", "--name", scheduler, "--network", "host", "--pid", "host", "--cap-add", "SYS_PTRACE",
                      "--security-opt", "apparmor=unconfined",
                      "--env", "DAGU_HOME=/var/lib/dagu", "--env", "DAGU_HOST=127.0.0.1", "--env", "DAGU_PORT=" + str(port),
                      "--env", "FIXTURE_REGISTRY=" + fixture.registry_image,
                      "--mount", "type=bind,source=" + str(home) + ",target=/var/lib/dagu",
                      "--mount", "type=bind,source=" + str(control) + ",target=" + str(control),
                      "--mount", "type=bind,source=" + str(script_root) + ",target=" + str(script_root) + ",readonly",
                      "--mount", "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock",
                      "--mount", "type=bind,source=/var/lib/docker,target=/var/lib/docker,readonly"]
    for directory in (data, config):
        scheduler_args += ["--mount", "type=bind,source=" + str(directory) + ",target=" + str(directory) + ",readonly"]
    if scenario == "restart":
        scheduler_args += ["--env", "FIXTURE_HOLD_AFTER_REPLACE=1"]
    docker(*scheduler_args, "echo-noise-update:u6-1")
    try:
        eventually(lambda: http(dagu_url + "/api/v1/health")[0] == 200)
        password = secrets.token_hex(20)
        code, session = http(dagu_url + "/api/v1/auth/setup", "POST", {"username": "fixture-admin", "password": password})
        assert code == 200
        admin = session["token"]
        validation = subprocess.run(["docker", "exec", scheduler, "dagu", "validate", "/var/lib/dagu/dags/" + task_name + ".yaml"], capture_output=True, text=True)
        assert validation.returncode == 0, (validation.stdout, validation.stderr)
        code, webhook = http(dagu_url + "/api/v1/dags/" + task_name + "/webhook", "POST", token=admin)
        assert code == 201 or code == 200, (code, webhook)
        executor.atomic_write(config / "wake-token", webhook["token"])
        assert http(wake_url, "POST", token="wrong-token")[0] == 401
        docker("exec", scheduler, "dagu", "validate", "/var/lib/dagu/dags/" + task_name + ".yaml")
        check_args = ["docker", "exec", scheduler, "python3", str(script_root / "fixture/container-executor.py"), "check", str(control / "executor.json")]
        def check():
            result = None
            def invoke():
                nonlocal result
                result = subprocess.run(check_args, capture_output=True, text=True)
                return "executor_already_running" not in result.stderr
            eventually(invoke)
            return result
        # The real minute schedule may already own flock at this exact second.
        # Retry only lock contention; every actual check result is still asserted.
        initial = check()
        assert initial.returncode == 0, initial.stderr
        before = http("http://127.0.0.1:1314/fixture/state")[1]["data"]["executor"]["checked_at"]
        # No task exists. A later check must come from the actual minute schedule.
        eventually(lambda: http("http://127.0.0.1:1314/fixture/state")[1]["data"]["executor"]["checked_at"] != before, 100)
        assert readiness()
        assert docker("inspect", app, "--format", "{{.Id}}") == old
        print("Dagu " + scenario + ": authenticated webhook rejects wrong token; scheduled idle check preserves old instance", flush=True)
        if scenario == "wake":
            writer = docker("run", "-d", "--name", fixture.prefix + "-writer", "--mount",
                            "type=bind,source=" + str(data) + ",target=/data", "--entrypoint", "/bin/sh", refs[0][0], "-c", "sleep 120")
            try:
                result = check()
                assert result.returncode != 0 and "another_container_can_write_data" in result.stderr, result.stderr
            finally:
                docker("rm", "-f", writer)
            with open(data / "sentinel", "rb"):
                result = check()
                assert result.returncode != 0 and "host_process_can_write_data" in result.stderr, result.stderr
            result = check()
            assert result.returncode == 0, result.stderr
            print("Dagu: real host process and another writable container both block installation", flush=True)
        if scenario == "verify-failure":
            (data / "fail-health").touch()
        code, created = http("http://127.0.0.1:1314/fixture/create", "POST")
        assert code == 201
        task = created["data"]["id"]
        if scenario == "wake":
            with concurrent.futures.ThreadPoolExecutor(2) as pool:
                list(pool.map(lambda _: http(wake_url, "POST", token=webhook["token"]), range(2)))
        if scenario == "restart":
            eventually(lambda: (control / "state/after-replace").exists())
            if restart_second is not None:
                eventually(lambda: int(time.time()) % 60 == restart_second, 65)
            installed = docker("inspect", app, "--format", "{{.Id}}")
            logs = [file.relative_to(home) for file in (home / "logs").rglob("*") if file.is_file()]
            assert logs
            docker("stop", "--time", "2", scheduler)
            docker("rm", scheduler)
            restart_at = time.monotonic()
            docker(*scheduler_args, "echo-noise-update:u6-1")
            eventually(lambda: http(dagu_url + "/api/v1/health")[0] == 200)
            assert http(dagu_url + "/api/v1/dags", token=admin)[0] == 200
            assert (control / "state/active.json").exists()
            assert all((home / file).exists() for file in logs)
        status = "needs_attention" if scenario == "verify-failure" else "succeeded"
        # A killed Dagu run retains a fresh heartbeat and max_active_runs slot.
        # Measured NAS recovery crossed the old 130s bound, then completed at
        # 133s without intervention. Allow 90s stale + 3*45s detection + 60s
        # minute dispatch + startup; do not alter Dagu or executor protection.
        eventually(lambda: http("http://127.0.0.1:1314/fixture/tasks/" + task)[1]["data"]["status"] == status,
                   300 if scenario == "restart" else 130)
        journal = control / "state/active.json"
        if not journal.exists():
            journal = control / ("state/" + task + ".json")
        record = json.loads(journal.read_text())
        assert record["task"]["id"] == task and record["backup_complete"]
        new_id = docker("inspect", app, "--format", "{{.Id}}")
        assert new_id != old
        if scenario == "restart":
            assert new_id == installed, "recovery replaced twice"
            assert record["step"] == "complete" and record["closed"] and not record["pending"]
            old_state = json.loads(docker("inspect", old))[0]
            assert not old_state["State"]["Running"] and old_state["HostConfig"]["RestartPolicy"]["Name"] == "no"
            db = sqlite3.connect((data / "fixture.db").as_uri() + "?mode=ro", uri=True)
            try:
                events = [row[0] for row in db.execute("SELECT status FROM update_task_events ORDER BY id")]
                assert events == ["claimed", "downloading", "stopping", "backing_up", "replacing", "verifying", "succeeded"], events
                assert db.execute("SELECT content FROM messages WHERE id=1").fetchone()[0] == "old WAL note"
                assert db.execute("SELECT count(*) FROM update_tasks WHERE active_slot=1").fetchone()[0] == 0
            finally:
                db.close()
            print("Dagu restart: recovery seconds=" + str(round(time.monotonic() - restart_at, 2)) +
                  " trigger_second=" + str(restart_second) + " target=" + new_id +
                  "; ordered events and unchanged target checked", flush=True)
            before = http("http://127.0.0.1:1314/fixture/state")[1]["data"]["executor"]["checked_at"]
            eventually(lambda: http("http://127.0.0.1:1314/fixture/state")[1]["data"]["executor"]["checked_at"] != before, 100)
            assert readiness()
            assert (control / ("state/" + task + ".json")).exists(), "closed journal not archived on next minute"
        with zipfile.ZipFile(Path(record["backup_path"]) / "backup.zip") as archive:
            assert archive.read("protected-config/runtime.env") == (config / "runtime.env").read_bytes()
            assert "database.db" in archive.namelist()
        assert (data / "sentinel").read_bytes() == b"preserved attachment"
        if scenario == "wake":
            assert http("http://127.0.0.1:1314/fixture/revoke", "POST")[0] == 200
            rejected = subprocess.run(["docker", "exec", scheduler, "python3", str(script_root / "fixture/container-executor.py"), "check", str(control / "executor.json")], capture_output=True, text=True)
            assert rejected.returncode != 0 and "http_401" in rejected.stderr
        print("Dagu " + scenario + ": task=" + task + " status=" + status + "; real backup, replacement, persistence and data checked", flush=True)
    finally:
        evidence = os.environ.get("U7_DAGU_EVIDENCE")
        if evidence:
            destination = Path(evidence) / (fixture.prefix + "-" + root.name + "-" + scenario)
            destination.mkdir(mode=0o700, parents=True, exist_ok=True)
            for directory in (control / "state", home):
                shutil.copytree(directory, destination / directory.name, dirs_exist_ok=True)
        if os.sys.exc_info()[0] is not None:
            print(docker("logs", "--tail", "35", scheduler), flush=True)
            diagnostic = subprocess.run(["docker", "exec", scheduler, "python3", str(script_root / "fixture/container-executor.py"), "check", str(control / "executor.json")], capture_output=True, text=True)
            print("isolated executor diagnostic:", diagnostic.stdout, diagnostic.stderr, flush=True)
            for logfile in (home / "logs").rglob("*"):
                if logfile.is_file() and logfile.suffix in (".log", ".out", ".err") and "echo-noise-update" in str(logfile):
                    print("isolated task log:", logfile.read_text(errors="replace")[-3000:], flush=True)
        for cid in docker("ps", "-aq", "--filter", "name=" + fixture.prefix + "-" + scenario).split():
            docker("rm", "-f", cid)
        with contextlib.suppress(executor.Stop):
            docker("rm", "-f", scheduler)


if __name__ == "__main__":
    fixture.prefix = "echo-noise-u6-" + str(os.getpid())
    fixture.containers = []
    with tempfile.TemporaryDirectory(prefix=fixture.prefix) as directory:
        fixture.root = Path(directory)
        try:
            refs = fixture.build_images()
            for scenario in ("wake", "missed", "verify-failure"):
                test_case(fixture.root, refs, scenario)
            for second in (1, 31, 45):
                case_root = fixture.root / ("restart-at-" + str(second))
                case_root.mkdir(mode=0o700)
                test_case(case_root, refs, "restart", second)
        finally:
            for cid in docker("ps", "-aq", "--filter", "name=" + fixture.prefix).split():
                docker("rm", "-f", cid)
