#!/usr/bin/env python3
"""Real engine/registry + real TaskService/auth/controllers, empty fixture data only.

Run from the repo root after building Linux coordinator-1 and coordinator-2.
No personal NAS settings, production data or production route switches.
"""
import contextlib
import json
import copy
import multiprocessing
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
    def inspect(self, reference):
        result = super().inspect(reference)
        if str(reference).endswith("-update-check") or result.get("Name", "").endswith("-update-check"):
            self.probe = result
        return result

    def preflight(self):
        try:
            return super().preflight()
        except executor.Stop:
            # Fixture-only diagnostics: no personal env, credentials or production input.
            if hasattr(self, "probe"):
                current = self.inspect(self.container_id())["HostConfig"]
                expected = self.probe["HostConfig"]
                print("fixture HostConfig differences: " + json.dumps({k: [current.get(k), expected.get(k)] for k in current.keys() | expected.keys() if current.get(k) != expected.get(k) and k not in ("Binds", "Mounts")}), flush=True)
            raise

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


def rejected(action, code):
    try:
        action()
        raise AssertionError("accepted: " + code)
    except executor.Stop as error:
        assert str(error).startswith(code), str(error)


def killed_child(action):
    child = multiprocessing.Process(target=action)
    child.start()
    child.join(180)
    if child.is_alive():
        child.kill()
        child.join()
        raise AssertionError("executor did not reach interruption")
    assert child.exitcode == -signal.SIGKILL


def test_paths(ex):
    control = root / "permissions"
    control.mkdir(mode=0o700)
    token = control / "token"
    executor.atomic_write(token, "fixture")
    executor.private(token)
    os.chown(control, 1001, 1001)
    rejected(lambda: executor.private(token), "control_parent_owner")
    control.chmod(0o1777)
    rejected(lambda: executor.private(token), "control_parent_owner")
    os.chown(control, 0, 0)
    control.chmod(0o777)
    rejected(lambda: executor.private(token), "control_parent_writable")
    control.chmod(0o700)
    link = root / "control-link"
    link.symlink_to(control, target_is_directory=True)
    rejected(lambda: executor.private(link / "token"), "control_parent_symlink")
    link_file = root / "config-link"
    link_file.symlink_to(ex.config_path)
    rejected(lambda: executor.Executor(link_file), "missing_or_symlink_control_file")
    executor.private(token)  # root-owned sticky /tmp remains supported.
    sockets = ex.docker_sockets()
    directory = root / "socket-test"
    directory.mkdir()
    with socket.socket(socket.AF_UNIX) as local:
        local.bind(str(directory / "engine.sock"))
        alias = root / "engine-link"
        alias.symlink_to(directory / "engine.sock")
        original = copy.deepcopy(ex.cfg["mounts"])
        try:
            # Real local Engine socket, its aliases/parents, and a nonstandard endpoint.
            for endpoint in (sockets[0], directory / "engine.sock"):
                for source in (endpoint, endpoint.parent, alias if endpoint.parent == directory else Path("/var/run/docker.sock")):
                    ex.cfg["mounts"] = [{"type": "bind", "source": str(source), "target": "/host/renamed"}]
                    with unittest_patch.dict(os.environ, {"DOCKER_HOST": "unix://" + str(endpoint), "DOCKER_CONTEXT": ""}):
                        rejected(lambda: ex.check_mounts({"Mounts": [{"Type": "bind", "Source": str(source),
                                     "Destination": "/host/renamed", "RW": True}]}), "docker_socket_in_application")
        finally:
            ex.cfg["mounts"] = original
    print("F2/F6: real UID/mode/symlink and UNIX/Engine socket aliases/parents rejected", flush=True)


def test_docker_preflight(ex, args, cid, cfg, old_ref):
    network = prefix + "-extra"
    docker("network", "create", network)
    networks.append(network)
    docker("network", "connect", network, cid)
    rejected(ex.preflight, "docker_networks_not_represented")
    assert current_id(ex) == cid and ex.inspect(cid)["State"]["Running"]
    docker("network", "disconnect", network, cid)
    for index, extra in enumerate((["--hostname", "custom"], ["--domainname", "custom.test"], ["--network", "host"],
                                   ["--mac-address", "02:42:ac:11:00:77"])):
        cfg2 = copy.deepcopy(cfg)
        name = prefix + "-config-" + str(index)
        cfg2["container"] = name
        directory = root / ("config-" + str(index))
        directory.mkdir()
        cfg2["mounts"][0]["source"] = str(directory)
        changed = list(args)
        changed[changed.index("--name") + 1] = name
        changed[changed.index("--mount") + 1] = "type=bind,source=" + str(directory) + ",target=/data"
        if index != 2:
            changed[changed.index("--publish") + 1] = "127.0.0.1::1314"
            cfg2["docker"]["ports"] = ["127.0.0.1::1314"]
        if index == 2:
            # Host networking requires no published ports.
            del changed[changed.index("--publish"):changed.index("--publish") + 2]
            cfg2["docker"].pop("ports")
            cfg2["docker"]["network"] = "host"
        other = docker(*changed[:-1], *extra, old_ref)
        containers.append(other)
        docker("start", other)
        checked = new_executor(cfg2)
        if index == 2:
            checked.preflight()
        elif index == 3:
            assert checked.inspect(other)["Config"].get("MacAddress") == "02:42:ac:11:00:77"
            rejected(checked.preflight, "docker_mac_address_not_represented")
        else:
            rejected(checked.preflight, "docker_hostname_or_domain_not_represented")
        assert checked.inspect(other)["State"]["Running"]
        docker("rm", "--force", other)
    new_executor(cfg)  # Restore the original registered config file.
    # Commit create, then kill before its result reaches the cleanup boundary.
    def die_after_create():
        create = ex.docker_create
        def killed(*a, **k):
            create(*a, **k)
            os.kill(os.getpid(), signal.SIGKILL)
        ex.docker_create = killed
        ex.preflight()
    killed_child(die_after_create)
    probe = cfg["container"] + "-update-check"
    assert ex.inspect(probe)["State"]["Status"] == "created"
    ex.preflight()
    assert not docker("ps", "-aq", "--filter", "name=^/" + probe + "$")
    create = ex.docker_create
    def lost(*a, **k):
        create(*a, **k)
        raise executor.Stop("command_unavailable_or_timeout")
    with unittest_patch.object(ex, "docker_create", side_effect=lost):
        rejected(ex.preflight, "command_unavailable_or_timeout")
    ex.preflight()
    foreign = docker("create", "--name", probe, old_ref)
    rejected(ex.preflight, "probe_name_owned_by_other")
    assert ex.inspect(probe)["Id"] == foreign
    docker("rm", foreign)
    directory = root / "running-probe"
    directory.mkdir()
    labels = [item for k, v in ex.probe_labels().items() for item in ("--label", k + "=" + v)]
    running = docker("run", "-d", "--name", probe, *labels, "--mount", "type=bind,source=" + str(directory) + ",target=/data", old_ref)
    rejected(ex.preflight, "probe_has_run_requires_manual_reconciliation")
    assert ex.inspect(running)["State"]["Running"]
    docker("rm", "--force", running)  # Only fixture code cleans its deliberate running probe.
    assert current_id(ex) == cid and ex.inspect(cid)["State"]["Running"]
    print("F1/F5/MAC: extra network/custom identity/fixed MAC rejected; host passes; killed/lost probe recovered; foreign/running preserved", flush=True)


def test_engine_and_volumes(ex, old_ref):
    original = copy.deepcopy(ex.cfg["mounts"])
    cid = current_id(ex)
    remote_context = prefix + "-remote-" + ex.cfg["mode"]
    docker("context", "create", remote_context, "--docker", "host=tcp://127.0.0.1:1")
    try:
        ex.cfg["mounts"] = []
        for environment in ({"DOCKER_HOST": "tcp://127.0.0.1:1", "DOCKER_CONTEXT": ""},
                            {"DOCKER_CONTEXT": remote_context, "DOCKER_HOST": "unix:///ignored.sock"}):
            with unittest_patch.dict(os.environ, environment):
                rejected(ex.preflight, "local_unix_docker_endpoint_required")
    finally:
        ex.cfg["mounts"] = original
        docker("context", "rm", remote_context)
    endpoint = ex.docker_sockets()[0]
    for advanced in (False, True):
        name = prefix + "-volume-" + ex.cfg["mode"] + "-" + str(advanced).lower()
        options = ["--opt", "type=none", "--opt", "o=bind", "--opt", "device=" + str(endpoint.parent)] if advanced else []
        docker("volume", "create", *options, name)
        volumes.append(name)
        directory = root / name
        directory.mkdir()
        other = docker("run", "-d", "--name", name, "--mount", "type=bind,source=" + str(directory) + ",target=/data",
                       "--mount", "type=volume,source=" + name + ",target=/host/volume,readonly", old_ref)
        containers.append(other)
        actual = ex.inspect(other)
        try:
            ex.cfg["mounts"] = [{"type": "bind", "source": str(directory), "target": "/data"},
                                {"type": "volume", "source": name, "target": "/host/volume", "read_only": True}]
            if advanced:
                mounted = next(m for m in actual["Mounts"] if m["Type"] == "volume")
                assert os.path.samefile(Path(mounted["Source"]) / endpoint.name, endpoint)
                rejected(lambda: ex.check_mounts(actual), "named_volume_options_unsupported")
            else:
                ex.check_mounts(actual)
            assert ex.inspect(other)["State"]["Running"]
        finally:
            ex.cfg["mounts"] = original
            docker("rm", "--force", other)
    assert current_id(ex) == cid and ex.inspect(cid)["State"]["Running"]
    print(ex.cfg["mode"] + ": no-mount remote host/context rejected; real socket hidden by local volume options rejected; ordinary named volume passes", flush=True)


def test_compose_preflight(ex, model, compose, cid, other_id):
    for settings in ({"deploy": {"replicas": 2}}, {"scale": 2}):
        changed = copy.deepcopy(model)
        changed["services"]["app"].update(settings)
        executor.atomic_write(compose, json.dumps(changed))
        ex.compose("up", "-d", "--no-deps", "--no-recreate", "--scale", "app=1", "--pull", "never", "app")
        before = current_id(ex)
        rejected(ex.preflight, "compose_requires_single_replica")
        assert current_id(ex) == before == cid
        assert ex.compose("ps", "-q", "other").strip() == other_id
        assert ex.inspect(cid)["State"]["Running"]
    executor.atomic_write(compose, json.dumps(model))
    ex.preflight()
    print("F3: replicas=2/scale=2 overridden to one current container rejected; app/other IDs unchanged", flush=True)


def test_attention(ex, task, cid):
    ex.claim()
    ex.phase("download_intent", "downloading")
    ex.flush()
    ex.phase("stop_intent", "stopping")
    ex.flush()
    def die_after_save():
        save = ex.save
        def killed():
            save()
            os.kill(os.getpid(), signal.SIGKILL)
        ex.save = killed
        ex.attention("interrupted_destructive_step")
    killed_child(die_after_save)
    ex.load_record()
    assert ex.record["pending"] == ["needs_attention"] and ex.record["error_code"] == "interrupted_destructive_step"
    rejected(ex.run, "manual_reconciliation_required")
    assert_status(task["id"], "needs_attention")
    # Emulate the old incomplete journal; idempotent owned reporting reconciles it.
    ex.record.update(step="attention", confirmed="stopping", pending=[])
    ex.save()
    rejected(ex.run, "manual_reconciliation_required")
    assert ex.record["confirmed"] == "needs_attention" and not ex.record["closed"]
    assert current_id(ex) == cid and ex.inspect(cid)["State"]["Running"]
    occupied = seed(ex)
    assert occupied["id"] == task["id"] and occupied["status"] == "needs_attention"
    print("F4: SIGKILL after first attention write recovered to real DB needs_attention; legacy reconciled; active slot retained", flush=True)


def test_mode(mode, old_ref, new_ref, new_digest, attention=False):
    global data, url
    case = root / (mode + ("-attention" if attention else ""))
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
        name = prefix + "-docker" + ("-attention" if attention else "")
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
    if not attention:
        test_engine_and_volumes(ex, old_ref)
        if mode == "docker":
            test_paths(ex)
            test_docker_preflight(ex, args, cid, cfg, old_ref)
        else:
            test_compose_preflight(ex, model, compose, cid, other_id)
    # Independent flock invocations cannot both enter the same registered deployment.
    import fcntl
    with open(ex.state / "executor.lock", "a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        blocked = subprocess.run(["python3", str(Path(__file__).with_name("executor.py")), "claim", str(ex.config_path)], capture_output=True, text=True)
        assert blocked.returncode != 0 and "executor_already_running" in blocked.stderr
    task = seed(ex)
    if attention:
        test_attention(ex, task, cid)
        return
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
    from unittest.mock import patch as unittest_patch
    print("Engine " + docker("version", "--format", "{{.Server.Version}}") + "; " + docker("compose", "version", "--short"), flush=True)
    prefix = "echo-noise-u3-" + str(os.getpid())
    containers = []
    compose_cleanup = []
    networks = []
    volumes = []
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
            test_mode("docker", refs[0][0], refs[1][0], refs[1][1], attention=True)
        finally:
            for project, image_file, compose in compose_cleanup:
                with contextlib.suppress(executor.Stop):
                    docker("compose", "-p", project, "--env-file", str(image_file), "-f", str(compose), "down", "--volumes")
            # Cleanup only this fixture's registered containers; never prune the engine.
            ids = docker("ps", "-aq", "--filter", "name=" + prefix).split()
            for cid in ids:
                with contextlib.suppress(executor.Stop):
                    docker("rm", "--force", cid)
            for network in networks:
                with contextlib.suppress(executor.Stop):
                    docker("network", "rm", network)
            for volume in volumes:
                with contextlib.suppress(executor.Stop):
                    docker("volume", "rm", volume)
