"""Writer-guard regressions; real /proc cases run only on Linux."""
import ctypes
import importlib.util
import io
import mmap
import os
from pathlib import Path
import stat
import sys
import tempfile
import types
import unittest
from unittest.mock import MagicMock, patch

spec = importlib.util.spec_from_file_location("writer_executor", Path(__file__).with_name("executor.py"))
executor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(executor)


class WriterTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name).resolve()
        self.current = {"Id": "old", "State": {"Running": False},
                        "Mounts": [{"Source": str(self.root), "RW": True}]}
        self.ex = executor.Executor.__new__(executor.Executor)
        self.proc = MagicMock()
        self.proc.name = "12345"
        self.fd = MagicMock()
        self.fd.name = "42"
        self.fd.stat.return_value = types.SimpleNamespace(st_mode=stat.S_IFREG | 0o600)
        self.fds, self.infos, self.maps = MagicMock(), MagicMock(), MagicMock()
        self.proc.__truediv__.side_effect = {"fd": self.fds, "fdinfo": self.infos, "maps": self.maps}.__getitem__
        self.fds.iterdir.return_value = [self.fd]
        self.info = self.infos.__truediv__.return_value
        self.info.read_text.return_value = "flags:\t0100000\n"
        self.maps.open.side_effect = lambda: io.StringIO("")

    def check_fake(self):
        with patch.object(executor, "command", return_value=""), \
                patch.object(executor.os, "getpid", return_value=99999), \
                patch.object(Path, "iterdir", return_value=[self.proc]), \
                patch.object(executor.os, "readlink", return_value=str(self.root / "file")):
            self.ex.check_writers(self.current)

    def test_readonly_directory_and_regular_file(self):
        for kind in (stat.S_IFDIR, stat.S_IFREG):
            with self.subTest(kind=kind):
                self.fd.stat.return_value.st_mode = kind | 0o600
                self.check_fake()

    def test_path_only_directory_and_regular_file(self):
        # Linux O_PATH is 010000000, including on hosts whose Python lacks O_PATH.
        with patch.object(executor.os, "O_PATH", 0o10000000, create=True):
            self.info.read_text.return_value = "flags:\t012000000\n"
            for kind in (stat.S_IFDIR, stat.S_IFREG):
                with self.subTest(kind=kind):
                    self.fd.stat.return_value.st_mode = kind | 0o600
                    self.check_fake()

    def test_writable_file_modes_rejected_with_other_flag_bits(self):
        for flags in ("0102001", "02102002"):
            with self.subTest(flags=flags):
                self.info.read_text.return_value = "flags:\t" + flags + "\n"
                with self.assertRaisesRegex(executor.Stop, "host_process_can_write_data"):
                    self.check_fake()

    def test_missing_malformed_and_unknown_access_flags_fail_closed(self):
        for info in ("", "pos:\t0\n", "flags:\tgarbage\n", "flags:\t08\n",
                     "flags:\t0100003\n", "flags:\t040000000000\n", "flags:\t0\nflags:\t0\n"):
            with self.subTest(info=info):
                self.info.read_text.return_value = info
                with self.assertRaisesRegex(executor.Stop, "host_fd_flags_unavailable"):
                    self.check_fake()

    def test_permission_and_io_failures_fail_closed(self):
        for operation in (self.fds.iterdir, self.fd.stat, self.info.read_text, self.maps.open):
            for error, code in ((PermissionError, "root_required_to_check_writers"),
                                (OSError, "host_writer_inspection_failed")):
                with self.subTest(operation=operation, error=error):
                    operation.side_effect = error
                    try:
                        with self.assertRaisesRegex(executor.Stop, code):
                            self.check_fake()
                    finally:
                        operation.side_effect = (lambda: io.StringIO("")) if operation is self.maps.open else None

    def test_disappearing_fd_and_process_are_tolerated(self):
        self.fd.stat.side_effect = FileNotFoundError
        self.check_fake()
        self.fd.stat.side_effect = None
        self.fds.iterdir.side_effect = FileNotFoundError
        self.proc.stat.side_effect = FileNotFoundError
        self.check_fake()
        self.fds.iterdir.side_effect = None
        self.proc.stat.side_effect = None
        self.info.read_text.side_effect = FileNotFoundError
        # FD exists at the first stat, then disappears when fdinfo is missing.
        self.fd.stat.side_effect = [types.SimpleNamespace(st_mode=stat.S_IFREG), FileNotFoundError]
        self.check_fake()
        self.info.read_text.side_effect = None
        self.fd.stat.side_effect = None
        self.maps.open.side_effect = FileNotFoundError
        self.proc.stat.side_effect = FileNotFoundError
        self.check_fake()

    def test_missing_fd_directory_for_live_process_fails_closed(self):
        self.fds.iterdir.side_effect = FileNotFoundError
        with self.assertRaisesRegex(executor.Stop, "host_writer_inspection_failed"):
            self.check_fake()
        self.proc.stat.side_effect = PermissionError
        with self.assertRaisesRegex(executor.Stop, "root_required_to_check_writers"):
            self.check_fake()
        self.proc.stat.side_effect = OSError
        with self.assertRaisesRegex(executor.Stop, "host_writer_inspection_failed"):
            self.check_fake()

    def test_missing_fdinfo_or_maps_for_live_objects_fail_closed(self):
        self.info.read_text.side_effect = FileNotFoundError
        with self.assertRaisesRegex(executor.Stop, "host_fd_flags_unavailable"):
            self.check_fake()
        self.info.read_text.side_effect = None
        self.maps.open.side_effect = FileNotFoundError
        with self.assertRaisesRegex(executor.Stop, "host_process_maps_unavailable"):
            self.check_fake()

    def test_mappings_without_fds_and_paths_with_spaces_or_deleted_suffix(self):
        self.fds.iterdir.return_value = []
        for perms, rejected in (("rw-s", True), ("r--s", False), ("rw-p", False)):
            for suffix in ("", " (deleted)"):
                with self.subTest(perms=perms, suffix=suffix):
                    line = f"1000-2000 {perms} 00000000 00:01 42 {self.root}/file with spaces{suffix}\n"
                    self.maps.open.side_effect = lambda: io.StringIO(line)
                    if rejected:
                        with self.assertRaisesRegex(executor.Stop, "host_process_can_write_data"):
                            self.check_fake()
                    else:
                        self.check_fake()

    def test_malformed_maps_fail_closed(self):
        self.maps.open.side_effect = lambda: io.StringIO("unreadable mapping\n")
        with self.assertRaisesRegex(executor.Stop, "host_process_maps_unavailable"):
            self.check_fake()


@unittest.skipUnless(sys.platform.startswith("linux") and Path("/proc/self/fdinfo").is_dir(),
                     "requires Linux procfs")
class LinuxWriterTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name).resolve()
        self.file = self.root / "mapped file with spaces"
        self.file.write_bytes(b"\0" * mmap.PAGESIZE)
        self.current = {"Id": "old", "State": {"Running": False},
                        "Mounts": [{"Source": str(self.root), "RW": True}]}
        self.ex = executor.Executor.__new__(executor.Executor)
        self.proc = Path("/proc") / str(os.getpid())

    def check_real(self):
        # Isolate this process, but use its real fd links, stat, fdinfo and maps.
        # The guard normally excludes itself; remove only that exclusion here.
        iterdir = Path.iterdir
        def isolated(path):
            return iter([self.proc]) if path == Path("/proc") else iterdir(path)
        with patch.object(Path, "iterdir", isolated), \
                patch.object(executor.os, "getpid", return_value=-1), \
                patch.object(executor, "command", return_value=""):
            self.ex.check_writers(self.current)

    def test_real_fd_types_and_flags(self):
        cases = [(self.root, os.O_RDONLY | os.O_DIRECTORY, False),
                 (self.file, os.O_RDONLY, False),
                 (self.file, os.O_WRONLY | os.O_APPEND, True),
                 (self.file, os.O_RDWR, True)]
        if hasattr(os, "O_PATH"):
            cases += [(self.root, os.O_PATH | os.O_DIRECTORY, False),
                      (self.file, os.O_PATH, False)]
        for path, flags, rejected in cases:
            with self.subTest(path=path, flags=flags):
                fd = os.open(path, flags)
                try:
                    if rejected:
                        with self.assertRaisesRegex(executor.Stop, "host_process_can_write_data"):
                            self.check_real()
                    else:
                        self.check_real()
                finally:
                    os.close(fd)
        self.check_real()

    def test_real_mappings_survive_closed_fd(self):
        # libc avoids Python mmap's duplicate FD, so only /proc/maps can detect it.
        libc = ctypes.CDLL(None, use_errno=True)
        libc.mmap.restype = ctypes.c_void_p
        libc.mmap.argtypes = [ctypes.c_void_p, ctypes.c_size_t, ctypes.c_int,
                              ctypes.c_int, ctypes.c_int, ctypes.c_long]
        libc.munmap.restype = ctypes.c_int
        libc.munmap.argtypes = [ctypes.c_void_p, ctypes.c_size_t]
        cases = [(mmap.PROT_READ, mmap.MAP_SHARED, False, False),
                 (mmap.PROT_READ | mmap.PROT_WRITE, mmap.MAP_PRIVATE, False, False),
                 (mmap.PROT_READ | mmap.PROT_WRITE, mmap.MAP_SHARED, True, False),
                 (mmap.PROT_READ | mmap.PROT_WRITE, mmap.MAP_SHARED, True, True)]
        for prot, flags, rejected, deleted in cases:
            with self.subTest(prot=prot, flags=flags, deleted=deleted):
                fd = os.open(self.file, os.O_RDWR)
                try:
                    address = libc.mmap(None, mmap.PAGESIZE, prot, flags, fd, 0)
                finally:
                    os.close(fd)
                if address == ctypes.c_void_p(-1).value:
                    raise OSError(ctypes.get_errno(), "mmap failed")
                try:
                    if deleted:
                        self.file.unlink()
                    if rejected:
                        with self.assertRaisesRegex(executor.Stop, "host_process_can_write_data"):
                            self.check_real()
                    else:
                        self.check_real()
                finally:
                    libc.munmap(address, mmap.PAGESIZE)
                self.check_real()


if __name__ == "__main__":
    unittest.main()
