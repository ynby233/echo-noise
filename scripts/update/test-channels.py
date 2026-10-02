#!/usr/bin/env python3
"""Real registry/artifacts/controllers/executor; GitHub event responses are local.

Run on an independent Linux Engine with U7_CHANNEL_FIXTURES containing a
three-commit git repository, revisions.json, built-at and coordinator-A/B/C.
The binaries must embed those revisions, versions and build time.
"""
import contextlib
import http.server
import importlib.util
import json
import os
from pathlib import Path
import shutil
import shlex
import signal
import sqlite3
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request
from unittest.mock import patch

import executor

spec = importlib.util.spec_from_file_location("engine_fixture", Path(__file__).with_name("test-docker.py"))
f = importlib.util.module_from_spec(spec)
spec.loader.exec_module(f)
f.unittest_patch = patch
docker = f.docker
source = Path(os.environ["U7_CHANNEL_FIXTURES"]).resolve()
scripts = Path("scripts").resolve()
revisions = json.loads((source / "revisions.json").read_text())
built_at = (source / "built-at").read_text().strip()
rebuilt_at = (source / "rebuilt-at").read_text().strip()
versions = ["v1.0.0", revisions[1][:12], "v1.1.0"]


def shell(script, *args, env=None):
    return subprocess.run(["bash", str(script), *args], cwd=source / "git", env=env,
                          capture_output=True, text=True, timeout=180)


def git(*args):
    return subprocess.check_output(["git", "-C", str(source / "git"), *args], text=True).strip()


def registry(tag, value=None):
    request = urllib.request.Request(registry_url + "/manifests/" + tag,
        data=json.dumps(value).encode() if value is not None else None,
        method="PUT" if value is not None else "GET",
        headers={"Content-Type": "application/vnd.oci.image.index.v1+json", "Accept": "application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.docker.distribution.manifest.v2+json"})
    with urllib.request.urlopen(request) as response:
        data = response.read()
        return json.loads(data) if data else None, response.headers["Docker-Content-Digest"]


def api(path="", method="GET", body=None, digest=""):
    request = urllib.request.Request("http://127.0.0.1:1314/fixture/channels" + path,
        method=method, data=json.dumps(body).encode() if body is not None else None,
        headers={"Content-Type": "application/json", "X-Fixture-Installed-Digest": digest})
    try:
        with urllib.request.urlopen(request) as response:
            return response.status, json.loads(response.read())["data"]
    except urllib.error.HTTPError as error:
        return error.code, None


class SourceAPI(http.server.BaseHTTPRequestHandler):
    mode = "published"
    conclusion = "success"
    latest = revisions[2]
    unknown = False

    def log_message(self, *args):
        pass

    def do_GET(self):
        path = self.path.removeprefix("/repos/ynby233/echo-noise")
        status = 200
        if self.path.startswith("/v2/"):
            request = urllib.request.Request("http://" + f.registry_image.split("/")[0] + self.path,
                                             headers={"Accept": self.headers.get("Accept", "application/vnd.oci.image.index.v1+json")})
            with urllib.request.urlopen(request) as response:
                self.send_response(response.status)
                for key in ("Content-Type", "Docker-Content-Digest"):
                    self.send_header(key, response.headers[key])
                self.end_headers(); self.wfile.write(response.read())
            return
        release = {"tag_name": "v1.1.0", "draft": self.mode == "draft", "prerelease": self.mode == "prerelease", "published_at": built_at}
        if path.startswith("/token"):
            value = {"token": "isolated-registry"}
        elif path == "/commits/main":
            value = {"sha": self.latest}
        elif path.startswith("/compare/"):
            before, after = path.split("/compare/", 1)[1].split("...")
            if self.unknown:
                status, value = 404, {}
            else:
                ahead = subprocess.run(["git", "-C", str(source / "git"), "merge-base", "--is-ancestor", before, after]).returncode == 0
                behind = subprocess.run(["git", "-C", str(source / "git"), "merge-base", "--is-ancestor", after, before]).returncode == 0
                value = {"status": "identical" if before == after else "ahead" if ahead else "behind" if behind else "diverged"}
        elif path.startswith("/actions/"):
            value = {"workflow_runs": [{"head_sha": self.latest, "status": "completed", "conclusion": self.conclusion}]}
        elif path == "/releases/latest" or path.startswith("/releases/tags/"):
            version = path.rsplit("/", 1)[-1]
            if version == "v1.0.0": release["tag_name"] = version
            status = 404 if self.mode == "deleted" else 200
            value = release
        elif path.startswith("/git/ref/tags/"):
            value = {"object": {"type": "tag", "sha": git("rev-parse", path.rsplit("/", 1)[-1])}}
        elif path.startswith("/git/tags/"):
            value = {"object": {"type": "commit", "sha": git("rev-parse", path.rsplit("/", 1)[-1] + "^{commit}")}}
        else:
            status, value = 404, {}
        self.send_response(status); self.send_header("Content-Type", "application/json"); self.end_headers()
        self.wfile.write(json.dumps(value).encode())


def workflow_step(name):
    # Execute the actual workflow shell, including its policy and registry reads.
    lines = Path(".github/workflows/docker-publish.yml").read_text().splitlines()
    start = lines.index("      - name: " + name)
    start += next(i for i, line in enumerate(lines[start:]) if line == "        run: |") + 1
    body = []
    for line in lines[start:]:
        if line and not line.startswith("          "): break
        body.append(line[10:])
    return "\n".join(body) + "\n"


with tempfile.TemporaryDirectory(prefix="echo-noise-u7-channels-") as temp:
    root = Path(temp); root.chmod(0o700)
    f.prefix = root.name; f.containers = []
    server = None
    app = None
    try:
        rid = docker("run", "-d", "--name", f.prefix + "-registry", "-p", "127.0.0.1::5000", "registry:2")
        port = json.loads(docker("inspect", rid))[0]["NetworkSettings"]["Ports"]["5000/tcp"][0]["HostPort"]
        f.registry_image = "127.0.0.1:" + port + "/ynby233/echo-noise"
        registry_url = "http://" + f.registry_image.split("/")[0] + "/v2/ynby233/echo-noise"
        indexes, digests = [], []
        for name, revision, version, timestamp in zip("ABCD", revisions + [revisions[2]], versions + [versions[2]], [built_at] * 3 + [rebuilt_at]):
            context = root / name; context.mkdir()
            for src, dst in ((source / ("coordinator-" + name), "coordinator"), (Path("update-tool"), "update-tool"),
                             (scripts / "update/fixture/Dockerfile", "Dockerfile"), (Path("docker-entrypoint.sh"), "docker-entrypoint.sh")):
                shutil.copyfile(src, context / dst)
            (context / "docker-entrypoint.sh").write_text((context / "docker-entrypoint.sh").read_text())
            for executable in ("coordinator", "update-tool"): (context / executable).chmod(0o755)
            tag = f.registry_image + ":candidate-" + name
            docker("build", "-t", tag, "--build-arg", "REVISION=" + revision, "--build-arg", "VERSION=" + version,
                   "--build-arg", "BUILT_AT=" + timestamp, str(context))
            docker("push", tag)
            manifest, _ = registry("candidate-" + name)
            # Docker's classic builder emits schema2. Buildx imagetools then
            # chooses a Docker list and drops OCI-only index annotations.
            # Publish an actual OCI platform manifest, matching the OCI release
            # fixture, rather than accepting metadata-free publication.
            manifest["mediaType"] = "application/vnd.oci.image.manifest.v1+json"
            manifest["config"]["mediaType"] = "application/vnd.oci.image.config.v1+json"
            for layer in manifest["layers"]:
                layer["mediaType"] = "application/vnd.oci.image.layer.v1.tar+gzip"
            raw = json.dumps(manifest).encode()
            request = urllib.request.Request(registry_url + "/manifests/oci-" + name, data=raw, method="PUT",
                                              headers={"Content-Type": manifest["mediaType"]})
            with urllib.request.urlopen(request) as response: digest = response.headers["Docker-Content-Digest"]
            manifest_size = len(raw)
            index = {"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json",
                     "annotations": {"org.opencontainers.image.revision": revision, "org.opencontainers.image.version": version,
                                     "org.opencontainers.image.created": timestamp, "org.opencontainers.image.source": "https://github.com/ynby233/echo-noise"},
                     "manifests": [{"mediaType": manifest["mediaType"], "digest": digest,
                                    "size": manifest_size, "platform": {"os": "linux", "architecture": "amd64"}}]}
            _, index_digest = registry("fixed-" + name, index)
            indexes.append(index); digests.append(index_digest)
            result = shell(scripts / "release/smoke-image.sh", f.registry_image + "@" + index_digest, revision, version, timestamp)
            assert result.returncode == 0, (result.stdout, result.stderr)
            print("channel artifact smoke", name, revision, index_digest, flush=True)
        registry("stable-mcp", indexes[0]); registry("edge-mcp", indexes[1])
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), SourceAPI)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        data, config, control = (root / name for name in ("data", "config", "control"))
        for folder in (data, config, control): folder.mkdir(mode=0o700)
        (data / "sentinel").write_bytes(b"same-instance-attachment")
        (config / "config.yaml").write_text("database:\n  type: sqlite\n  path: /data/fixture.db\n")
        executor.atomic_write(config / "runtime.env", "DB_PATH=/data/fixture.db\n")
        executor.atomic_write(control / "app.env", "FIXTURE_CHANNEL_PROXY=http://127.0.0.1:" + str(server.server_port) + "\n")
        executor.atomic_write(control / "image.env", "UPDATE_IMAGE=" + f.registry_image + "@" + digests[0] + "\n")
        mounts = [{"type": "bind", "source": str(folder), "target": target} for folder, target in ((data, "/data"), (data, "/app/data"), (config, "/app/config"))]
        app = f.prefix + "-app"
        opts = {"network": "host", "restart": "no", "env_file": str(control / "app.env")}
        args = ["run", "-d", "--name", app, "--network", "host", "--restart=no", "--env-file", opts["env_file"]]
        for mount in mounts: args += ["--mount", "type=bind,source=" + mount["source"] + ",target=" + mount["target"]]
        docker(*args, f.registry_image + "@" + digests[0])
        f.url = "http://127.0.0.1:1314"; f.wait()
        shutil.copyfile(data / "token", control / "token"); (control / "token").chmod(0o600)
        instance = (data / "instance").read_text()
        executor.atomic_write(control / "executor.json", json.dumps({"url": f.url, "instance_id": instance, "token_file": str(control / "token"),
            "state_dir": str(control / "state"), "backup_dir": str(control / "backups"), "image_file": str(control / "image.env"),
            "platform": "linux/amd64", "mode": "docker", "container": app, "mounts": mounts, "docker": opts, "min_free_bytes": 1024, "health_timeout": 10}))
        for channel, expected in (("edge", 1), ("stable", 2)):
            if channel == "stable": registry("stable-mcp", indexes[2])
            ex = f.IsolatedExecutor(control / "executor.json"); ex.check_deployment()
            code, overview = api(); target = next(c for c in overview["report"]["channels"] if c["name"] == channel)
            assert code == 200 and target["status"] == "update_available" and target["revision"] == revisions[expected]
            assert api("/preference", "PUT", {"channel": channel})[0] == 200
            code, task = api("/install", "POST", {"channel": channel, "revision": target["revision"], "digest": target["digest"]})
            assert code == 201 and task["target_digest"] == digests[expected]
            ex.run(); assert ex.record["closed"] and ex.record["confirmed"] == "succeeded"
            assert ex.runtime()["revision"] == revisions[expected] and (data / "instance").read_text() == instance
            assert (data / "sentinel").read_bytes() == b"same-instance-attachment"
            old = json.loads(docker("inspect", ex.record["old_container"]))[0]
            assert not old["State"]["Running"] and old["HostConfig"]["RestartPolicy"]["Name"] == "no"
            db = sqlite3.connect((data / "fixture.db").as_uri() + "?mode=ro", uri=True)
            try:
                assert db.execute("SELECT content FROM messages WHERE id=1").fetchone()[0] == "old WAL note"
                events = [row[0] for row in db.execute("SELECT e.status FROM update_task_events e JOIN update_tasks t ON t.id=e.task_id WHERE t.public_id=? ORDER BY e.id", (task["id"],))]
                assert events == ["claimed", "downloading", "stopping", "backing_up", "replacing", "verifying", "succeeded"], events
                assert db.execute("SELECT count(*) FROM update_tasks WHERE active_slot=1").fetchone()[0] == 0
            finally: db.close()
            ex.archive_closed()
            print("channels installed", channel, task["id"], revisions[expected], digests[expected], flush=True)
        installed = docker("inspect", app, "--format", "{{.Id}}")
        def no_install(channel, status, digest=""):
            ex = f.IsolatedExecutor(control / "executor.json"); ex.check_deployment()
            target = next(c for c in api(digest=digest)[1]["report"]["channels"] if c["name"] == channel)
            assert target["status"] == status and not target["has_update"], target
            assert api("/preference", "PUT", {"channel": channel})[0] == 200
            assert api("/install", "POST", {"channel": channel}, digest=digest)[0] == 409
            assert docker("inspect", app, "--format", "{{.Id}}") == installed
            print("channels refused", channel, status, flush=True)
        registry("stable-mcp", indexes[0]); no_install("stable", "channel_behind")
        for channel in ("stable", "edge"):
            registry(channel + "-mcp", indexes[2]); no_install(channel, "current")
        _, rebuilt_digest = registry("edge-mcp", indexes[3]); assert rebuilt_digest != digests[2]; no_install("edge", "current")
        no_install("edge", "same_source_rebuild", digests[2])
        for mode in ("draft", "prerelease", "deleted"):
            SourceAPI.mode = mode; no_install("stable", "invalid_target")
        SourceAPI.mode = "published"; SourceAPI.unknown = True
        registry("edge-mcp", indexes[0]); no_install("edge", "check_failed")
        SourceAPI.unknown = False
        git("switch", "-qc", f.prefix + "-fork", revisions[0]); (source / "git/fork").write_text("fork"); git("add", "fork"); git("commit", "-qm", "fork")
        fork = json.loads(json.dumps(indexes[1])); fork["annotations"]["org.opencontainers.image.revision"] = git("rev-parse", "HEAD")
        registry("edge-mcp", fork); no_install("edge", "diverged"); git("switch", "main")
        # Real workflow stages: fixed target survives cancellations/failure; channel
        # advances only after final smoke, and an older late completion is kept out.
        link = source / "git/scripts"
        if not link.exists(): link.symlink_to(scripts, target_is_directory=True)
        stages = {name: root / (name.replace(" ", "-") + ".sh") for name in ("Publish immutable target", "Move channel tag")}
        for name, path in stages.items(): path.write_text(workflow_step(name))
        registry("edge-mcp", indexes[1])
        for conclusion, expected in (("failure", "build_failed"), ("cancelled", "build_cancelled"), ("success", "publishing")):
            SourceAPI.conclusion = conclusion
            report = api()[1]["report"]
            assert report["latest_source"]["status"] == expected, report["latest_source"]
            assert not next(c for c in report["channels"] if c["name"] == "edge")["has_update"]
            assert registry("edge-mcp")[1] == digests[1]
            print("source publication incomplete", conclusion, expected, flush=True)
        env = dict(os.environ, CHANNEL="edge", IMAGE=f.registry_image, GITHUB_REPOSITORY="ynby233/echo-noise", CANDIDATE=f.registry_image + ":fixed-C",
                   REVISION=revisions[2], VERSION=versions[2], BUILT_AT=built_at, GITHUB_OUTPUT=str(root / "outputs"),
                   CHANNEL_REF=f.registry_image + ":edge-mcp", TARGET_REF=f.registry_image + ":sha-" + revisions[2] + "-mcp",
                   TARGET_REVISION=revisions[2], TARGET_VERSION=versions[2])
        bad_context = root / "failed-build"; bad_context.mkdir()
        (bad_context / "Dockerfile").write_text("FROM alpine:3.22\nRUN exit 9\n")
        build = subprocess.run(["docker", "build", "-t", f.registry_image + ":failed", str(bad_context)], capture_output=True, timeout=120)
        assert build.returncode != 0 and registry("edge-mcp")[1] == digests[1]
        def cancel_at(stage, preceding="", moved=False):
            marker = root / "cancel-marker"
            if marker.exists(): marker.unlink()
            script = root / "cancel.sh"
            script.write_text("set -euo pipefail\n" + preceding + '\necho ready > "$U7_STAGE_MARKER"\nexec sleep 180\n')
            cancel_env = dict(env, U7_STAGE_MARKER=str(marker))
            process = subprocess.Popen(["bash", str(script)], cwd=source / "git", env=cancel_env,
                                       stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, start_new_session=True)
            try:
                deadline = time.monotonic() + 120
                while not marker.exists() and process.poll() is None and time.monotonic() < deadline: time.sleep(0.1)
                assert marker.exists(), process.stderr.read().decode() if process.poll() is not None else "stage did not reach cancellation point"
                os.killpg(process.pid, signal.SIGTERM); process.wait(timeout=10)
                assert process.returncode == -signal.SIGTERM
                assert registry("edge-mcp")[0]["annotations"]["org.opencontainers.image.revision"] == revisions[2 if moved else 1]
                print("release process cancelled", stage, "channel", "published" if moved else "old", flush=True)
            finally:
                if process.poll() is None: os.killpg(process.pid, signal.SIGKILL); process.wait()
                process.stderr.close()
        cancel_at("before immutable publication")
        result = shell(stages["Publish immutable target"], env=env); assert result.returncode == 0, (result.stdout, result.stderr)
        assert registry("edge-mcp")[1] == digests[1]
        cancel_at("after immutable publication", "bash " + shlex.quote(str(stages["Publish immutable target"])))
        # A rejected final smoke is an actual failure before channel mutation.
        result = shell(scripts / "release/smoke-image.sh", env["TARGET_REF"], revisions[0], versions[0])
        assert result.returncode != 0 and registry("edge-mcp")[1] == digests[1]
        print("release fixed artifact published; cancel before move and failed smoke retain old channel", flush=True)
        result = shell(scripts / "release/smoke-image.sh", env["TARGET_REF"], revisions[2], versions[2]); assert result.returncode == 0, (result.stdout, result.stderr)
        cancel_at("after final smoke before channel move", "sh " + " ".join(map(shlex.quote,
                  [str(scripts / "release/smoke-image.sh"), env["TARGET_REF"], revisions[2], versions[2]])))
        result = shell(stages["Move channel tag"], env=env); assert result.returncode == 0, (result.stdout, result.stderr)
        moved = registry("edge-mcp")[1]
        published = registry("edge-mcp")[0]
        assert published.get("annotations", {}).get("org.opencontainers.image.revision") == revisions[2], published
        cancel_at("after channel move", "bash " + shlex.quote(str(stages["Move channel tag"])), moved=True)
        env.update(TARGET_REF=f.registry_image + ":fixed-B", TARGET_REVISION=revisions[1], TARGET_VERSION=versions[1])
        result = shell(stages["Move channel tag"], env=env); assert result.returncode == 0 and registry("edge-mcp")[1] == moved
        result = shell(scripts / "release/resolve-release.sh", "v1.1.0", "main"); assert result.returncode == 0 and revisions[2] in result.stdout
        print("release late old completion kept; annotated tag resolved; cancellation after move retains published target", flush=True)
    finally:
        if sys.exc_info()[0] is not None and app:
            logs = subprocess.run(["docker", "logs", "--tail", "25", app], capture_output=True, text=True)
            print(logs.stdout + logs.stderr, flush=True)
        evidence = os.environ.get("U7_CHANNEL_EVIDENCE")
        if evidence and app:
            destination = Path(evidence) / f.prefix
            destination.mkdir(mode=0o700, parents=True, exist_ok=True)
            for name in ("state", "backups"):
                if (control / name).exists(): shutil.copytree(control / name, destination / name)
            if (data / "fixture.db").exists():
                db = sqlite3.connect((data / "fixture.db").as_uri() + "?mode=ro", uri=True)
                copy = sqlite3.connect(destination / "tasks.db")
                try: db.backup(copy)
                finally: copy.close(); db.close()
        if server: server.shutdown(); server.server_close()
        for cid in docker("ps", "-aq", "--filter", "name=" + f.prefix).split():
            with contextlib.suppress(executor.Stop): docker("rm", "-f", cid)
