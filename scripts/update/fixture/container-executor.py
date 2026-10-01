"""Isolated registry adapter, never copied into the delivered executor image."""
import importlib.util
import os
from pathlib import Path
import sys
import time
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import executor

spec = importlib.util.spec_from_file_location("engine_fixture", Path(__file__).resolve().parents[1] / "test-docker.py")
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)
fixture.unittest_patch = mock.patch
fixture.registry_image = os.environ["FIXTURE_REGISTRY"]


class ContainerExecutor(fixture.IsolatedExecutor):
    def replace(self):
        super().replace()
        if os.environ.get("FIXTURE_HOLD_AFTER_REPLACE") == "1":
            (self.state / "after-replace").touch()
            while True:
                time.sleep(1)


executor.Executor = ContainerExecutor
sys.exit(executor.main())
