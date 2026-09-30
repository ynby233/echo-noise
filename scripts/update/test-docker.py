#!/usr/bin/env python3
"""Real engine/registry + real TaskService/auth/controllers, empty fixture data only.

Run from the repo root after building Linux coordinator-1 and coordinator-2.
No personal NAS settings, production data or production route switches.
"""
import contextlib
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request

import executor


class IsolatedExecutor(executor.Executor):
    def reference(self):
        # Only the fixture maps the official TaskService repository to a loopback registry.
        return registry_image + "@" + self.record["task"]["target_digest"]

    def manifest(self, reference):
        return json.loads(executor.command(["docker", "manifest", "inspect", "--insecure", "--verbose", reference]))

    def data_protection_available(self):
        return True

    def backup(self):
        # Explicit U3 empty-data simulation. Never imported by the production entry point.
        assert not self.inspect(self.record["old_container"])["State"]["Running"]
        dst = Path(self.record["backup_path"])
        shutil.copytree(data, dst)
        (dst / "U3-EMPTY-FIXTURE-ONLY").write_text("U4 must replace this with validated data protection.\n")


def docker(*args):
    return executor.command(["docker", *args], timeout=600).strip()


def request(path, method="POST"):
    with urllib.request.urlopen(urllib.request.Request(url + path, method=method), timeout=5) as response:
        return json.loads(response.read()) if response.headers.get("Content-Type", "").startswith("application/json") else None


def wait():
    for _ in range(90):
        try:
            request("/health", "GET")
            return
        except OSError:
            time.sleep(1)
    raise AssertionError("fixture server did not start")


def new_executor(cfg):
    config = root / "executor.json"
    executor.atomic_write(config, json.dumps(cfg))
    return IsolatedExecutor(config)


def current_id(ex):
    return ex.inspect(ex.container_id())["Id"]


def seed(ex):
    ex.runtime()  # Authenticate through the real middleware before TaskService.Create.
    return request("/fixture/create")["data"]


def assert_status(task_id, status):
    assert request("/fixture/tasks/" + task_id, "GET")["data"]["status"] == status


def test_mode(mode, old_ref, new_ref, new_digest):
    global data, url
    case = root / mode
    data = case / "data"
    data.mkdir(parents=True, mode=0o700)
    (data / "sentinel").write_text("preserved")
    image_file = case / "image.env"
    executor.write_image(image_file, old_ref)
    with socket.socket() as reservation:
        reservation.bind(("127.0.0.1", 0))
        app_port = reservation.getsockname()[1]
    publish = "127.0.0.1:" + str(app_port) + ":1314"
    opts = {"restart": "no", "ports": [publish], "env_file": str(case / "app.env"),
            "log_driver": "json-file", "log_options": {"max-size": "10m", "max-file": "3"}}
    executor.atomic_write(case / "app.env", "FIXTURE_TARGET_DIGEST=" + new_digest + "\nFIXTURE_TARGET_REVISION=" + "2" * 40 + "\n")
    cfg = {"url": "http://127.0.0.1:1", "instance_id": "0" * 32,
           "token_file": str(case / "token"), "state_dir": str(case / "state"),
           "backup_dir": str(case / "backups"), "platform": "linux/amd64", "mode": mode,
           "mounts": [{"type": "bind", "source": str(data), "target": "/data"}],
           "image_file": str(image_file), "min_free_bytes": 1024}
    # Config is created after the fixture emits its first one-time credential.
    if mode == "docker":
        name = prefix + "-docker"
        cfg.update(container=name, docker=opts)
        args = ["create", "--name", name, "--platform", "linux/amd64", "--restart", "no", "--env-file", str(case / "app.env"),
                "--publish", publish, "--mount", "type=bind,source=" + str(data) + ",target=/data",
                "--log-driver", "json-file", "--log-opt", "max-size=10m", "--log-opt", "max-file=3", old_ref]
        cid = docker(*args)
        containers.append(cid)
        docker("start", cid)
        other_id = None
    else:
        compose = case / "compose.json"
        model = {"services": {
            "app": {"image": "${UPDATE_IMAGE}", "env_file": [str(case / "app.env")], "ports": [publish],
                    "volumes": [str(data) + ":/data"], "restart": "no"},
            "other": {"image": old_ref, "volumes": [prefix + "-other:/data"], "restart": "no"}},
                 "volumes": {prefix + "-other": {"name": prefix + "-other"}}}
        executor.atomic_write(compose, json.dumps(model))
        cfg.update(compose_file=str(compose), project=prefix, service="app")
        docker("compose", "-p", prefix, "--env-file", str(image_file), "-f", str(compose), "up", "-d", "--pull", "never")
        compose_cleanup.append((prefix, image_file, compose))
        cid = docker("compose", "-p", prefix, "--env-file", str(image_file), "-f", str(compose), "ps", "-q", "app")
        other_id = docker("compose", "-p", prefix, "--env-file", str(image_file), "-f", str(compose), "ps", "-q", "other")
        other_mounts = json.loads(docker("inspect", other_id))[0]["Mounts"]
    port = json.loads(docker("inspect", cid))[0]["NetworkSettings"]["Ports"]["1314/tcp"][0]["HostPort"]
    url = "http://127.0.0.1:" + port
    cfg["url"] = url
    wait()
    shutil.copyfile(data / "token", cfg["token_file"])
    Path(cfg["token_file"]).chmod(0o600)
    cfg["instance_id"] = (data / "instance").read_text()
    ex = new_executor(cfg)
    ex.preflight()
    # Independent flock invocations cannot both enter the same registered deployment.
    import fcntl
    with open(ex.state / "executor.lock", "a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        blocked = subprocess.run(["python3", str(Path(__file__).with_name("executor.py")), "claim", str(ex.config_path)], capture_output=True, text=True)
        assert blocked.returncode != 0 and "executor_already_running" in blocked.stderr
    task = seed(ex)
    # Lose a committed claim response, then retrieve precisely the same task.
    assigned = ex.api("POST", "/api/updates/executor/claim")
    assert assigned["id"] == task["id"]
    ex.claim()
    ex.cfg["platform"] = "linux/arm64"
    try:
        ex.download()
        raise AssertionError("wrong registry architecture accepted")
    except executor.Stop as error:
        assert str(error) == "registry_platform_unavailable"
    ex.cfg["platform"] = "linux/amd64"
    (data / "drop-final").touch()
    if mode == "docker":
        # Real coordinator rejects out-of-order events, local pending record stays intact.
        ex.queue("verifying")
        try:
            ex.flush()
            raise AssertionError("409 was accepted")
        except executor.Stop as error:
            assert str(error) == "http_409"
        assert ex.record["pending"] == ["verifying"]
        # The fixture created this deliberate invalid event; remove it only in this test.
        ex.record["pending"] = []
        ex.save()
    if mode == "compose":
        # Kill the host executor after replacement, before it records the result.
        # On restart, inspect the real container and never repeat compose up.
        import multiprocessing
        def killed_run():
            normal_phase = ex.phase
            def phase(step, status=None):
                if step == "replaced":
                    os.kill(os.getpid(), signal.SIGKILL)
                normal_phase(step, status)
            ex.phase = phase
            ex.run()
        child = multiprocessing.Process(target=killed_run)
        child.start()
        child.join(180)
        if child.is_alive():
            child.kill()
            child.join()
            raise AssertionError("killed executor did not reach replacement")
        assert child.exitcode == -signal.SIGKILL
        ex.load_record()
        assert ex.record["step"] == "replace_intent"
        assert ex.target_running()
    try:
        ex.run()
        raise AssertionError("lost final response was treated as success")
    except executor.Stop as error:
        assert str(error) == "http_transport", str(error)
    assert_status(task["id"], "succeeded")
    new_id = current_id(ex)
    assert new_id != cid
    assert ex.record["step"] == "complete" and ex.record["pending"] == ["succeeded"]
    assert (data / "sentinel").read_text() == "preserved"
    assert Path(ex.record["backup_path"], "U3-EMPTY-FIXTURE-ONLY").exists()
    assert Path(cfg["image_file"]).read_text().strip() == "UPDATE_IMAGE=" + new_ref
    assert ex.record["download"]["requested_digest"] != ex.record["download"]["manifest_digest"]
    assert ex.record["download"]["image_id"] != ex.record["download"]["requested_digest"]
    # Rotate after final commit: old token may retry only its own final report.
    request("/fixture/rotate")
    recovered = new_executor(cfg)
    recovered.load_record()
    recovered.run()
    assert recovered.record["closed"]
    assert current_id(recovered) == new_id
    try:
        recovered.runtime()
        raise AssertionError("rotated terminal token gained runtime access")
    except executor.Stop as error:
        assert str(error) == "http_401"
    request("/fixture/revoke")
    recovered.record["closed"] = False
    recovered.record["confirmed"] = "verifying"
    recovered.queue("succeeded")
    try:
        recovered.run()
        raise AssertionError("revoked token accepted")
    except executor.Stop as error:
        assert str(error) == "http_401"
    assert recovered.record["pending"] == ["succeeded"]
    if mode == "compose":
        recovered.compose("up", "--detach", "--no-deps", "--no-build", "--pull", "never", "app")
        assert current_id(recovered) == new_id
        assert docker("compose", "-p", prefix, "--env-file", str(image_file), "-f", str(compose), "ps", "-q", "other") == other_id
        assert json.loads(docker("inspect", other_id))[0]["Mounts"] == other_mounts
    print(mode + ": real stop/replacement/runtime verified; lost response/rotation/revocation/lock passed", flush=True)


if __name__ == "__main__":
    prefix = "echo-noise-u3-" + str(os.getpid())
    containers = []
    compose_cleanup = []
    with tempfile.TemporaryDirectory(prefix=prefix) as tmp:
        root = Path(tmp)
        try:
            registry = docker("run", "--detach", "--name", prefix + "-registry", "--publish", "127.0.0.1::5000", "registry:2")
            containers.append(registry)
            registry_port = json.loads(docker("inspect", registry))[0]["NetworkSettings"]["Ports"]["5000/tcp"][0]["HostPort"]
            registry_image = "127.0.0.1:" + registry_port + "/echo-noise-u3"
            refs = []
            for i in (1, 2):
                context = root / ("image-" + str(i))
                context.mkdir()
                shutil.copyfile("scripts/update/fixture/Dockerfile", context / "Dockerfile")
                shutil.copyfile("coordinator-" + str(i), context / "coordinator")
                (context / "coordinator").chmod(0o755)
                tag = registry_image + ":fixture-" + str(i)
                docker("build", "--build-arg", "REVISION=" + str(i) * 40, "--tag", tag, str(context))
                docker("push", tag)
                digest = json.loads(docker("image", "inspect", tag))[0]["RepoDigests"][0].split("@", 1)[1]
                refs.append((registry_image + "@" + digest, digest))
            # Real OCI index digest differs from the selected platform manifest and image ID.
            manifest_url = "http://127.0.0.1:" + registry_port + "/v2/echo-noise-u3/manifests/"
            with urllib.request.urlopen(urllib.request.Request(manifest_url + refs[1][1], headers={"Accept": "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json"})) as response:
                manifest = response.read()
                media_type = response.headers["Content-Type"]
            index = {"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json", "manifests": [
                {"mediaType": media_type, "size": len(manifest), "digest": refs[1][1], "platform": {"os": "linux", "architecture": "amd64"}}]}
            with urllib.request.urlopen(urllib.request.Request(manifest_url + "fixture-2-index", data=json.dumps(index).encode(), method="PUT", headers={"Content-Type": index["mediaType"]})) as response:
                index_digest = response.headers["Docker-Content-Digest"]
            refs[1] = (registry_image + "@" + index_digest, index_digest)
            docker("pull", refs[1][0])
            test_mode("docker", refs[0][0], refs[1][0], refs[1][1])
            test_mode("compose", refs[0][0], refs[1][0], refs[1][1])
        finally:
            for project, image_file, compose in compose_cleanup:
                with contextlib.suppress(executor.Stop):
                    docker("compose", "-p", project, "--env-file", str(image_file), "-f", str(compose), "down", "--volumes")
            # Cleanup only this fixture's registered containers; never prune the engine.
            ids = docker("ps", "-aq", "--filter", "name=" + prefix).split()
            for cid in ids:
                with contextlib.suppress(executor.Stop):
                    docker("rm", "--force", cid)
