"""Cached image RepoDigests need not identify the just-pushed registry tag."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("engine_fixture", Path(__file__).with_name("test-docker.py"))
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)


class FixtureRegistryTests(unittest.TestCase):
    def test_cached_foreign_digest_does_not_select_manifest(self):
        repo = "127.0.0.1:12345/echo-noise-u3"
        manifest = "sha256:" + "a" * 64
        index = "sha256:" + "b" * 64

        def docker(*args):
            if args[0] == "inspect":
                return json.dumps([{"NetworkSettings": {"Ports": {"5000/tcp": [{"HostPort": "12345"}]}}}])
            if args[:2] == ("image", "inspect"):
                return json.dumps([{"RepoDigests": ["127.0.0.1:9999/echo-noise-u3@sha256:" + "0" * 64, repo + "@" + manifest]}])
            return "registry"

        class Response:
            headers = {"Docker-Content-Digest": manifest, "Content-Type": "application/vnd.docker.distribution.manifest.v2+json"}
            def __enter__(self): return self
            def __exit__(self, *args): pass
            def read(self): return b"{}"

        def urlopen(request):
            self.assertNotIn("sha256:" + "0" * 64, request.full_url)
            response = Response()
            if request.get_method() == "PUT":
                response.headers = {"Docker-Content-Digest": index}
            return response

        with tempfile.TemporaryDirectory() as tmp:
            fixture.root, fixture.prefix, fixture.containers = Path(tmp), "fixture-regression", []
            with patch.object(fixture, "docker", side_effect=docker), patch.object(fixture.shutil, "copyfile", side_effect=lambda _, dest: Path(dest).write_bytes(b"fixture")), patch.object(fixture.urllib.request, "urlopen", side_effect=urlopen):
                refs = fixture.build_images()
            self.assertEqual(refs, [(repo + "@" + manifest, manifest), (repo + "@" + index, index)])
