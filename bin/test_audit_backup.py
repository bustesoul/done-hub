"""Run with RCLONE_BIN=/path/to/rclone python3 -m unittest discover -s bin -p 'test_audit_backup.py'."""

import fcntl
import gzip
import importlib.util
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from types import SimpleNamespace
import unittest
from unittest import mock


SCRIPT = Path(__file__).with_name("audit-backup.py")
RCLONE = os.getenv("RCLONE_BIN") or shutil.which("rclone")


@unittest.skipUnless(RCLONE, "install rclone or set RCLONE_BIN for real local-backend regression tests")
class BackupRegression(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="audit backup test ")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.source = self.root / "archive"
        self.source.mkdir(mode=0o700)
        self.target = self.root / "remote target"
        self.segment = self.source / "audit-20261028T010000-unique.log"
        self.body = b'{"type":"request_body","data":"aGVsbG8="}\n'
        self.segment.write_bytes(self.body)
        self.environment = {**os.environ, "RCLONE_CONFIG": str(self.root / "empty-rclone.conf")}

    def command(self, rclone=None, *extra):
        return [sys.executable, str(SCRIPT), "--source", str(self.source),
                "--destination", str(self.target), "--allow-local", "--rclone", rclone or RCLONE, *extra]

    def run_backup(self, rclone=None, *extra):
        return subprocess.run(self.command(rclone, *extra), env=self.environment,
                              capture_output=True, text=True, timeout=30)

    def wrapper(self, code):
        path = self.root / "rclone-wrapper"
        path.write_text("#!" + sys.executable + "\nimport os,sys,time,shutil\n" + code +
                        "\nos.execv(" + repr(RCLONE) + ", [" + repr(RCLONE) + "] + sys.argv[1:])\n")
        path.chmod(0o700)
        return str(path)

    def assert_preserved(self, result):
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertTrue(self.segment.exists())
        self.assertEqual(list(self.source.glob(".backup-*.gz")), [])

    def test_verified_upload_removes_only_sealed_source(self):
        active = self.source / "audit-active.open"
        active.write_bytes(b"still being written")
        unrelated = self.source / "audit.log"
        unrelated.write_bytes(b"normal mode")
        result = self.run_backup()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.segment.exists())
        self.assertTrue(active.exists())
        self.assertTrue(unrelated.exists())
        compressed = self.target / (self.segment.name + ".gz")
        self.assertEqual(gzip.decompress(compressed.read_bytes()), self.body)
        self.assertEqual(compressed.stat().st_mode & 0o777, 0o600)
        self.assertIn('"source_removed": true', result.stdout)

    def test_keep_source_then_retry_is_idempotent(self):
        first = self.run_backup(None, "--keep-source")
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        self.assertEqual(self.segment.read_bytes(), self.body)
        remote = self.target / (self.segment.name + ".gz")
        uploaded = remote.read_bytes()
        second = self.run_backup()
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        self.assertFalse(self.segment.exists())
        self.assertEqual(remote.read_bytes(), uploaded)

    def test_environment_target_supports_named_rclone_remote(self):
        environment = {**self.environment, "AUDIT_BACKUP_DEST": "testremote:" + str(self.target),
                       "RCLONE_CONFIG_TESTREMOTE_TYPE": "local"}
        command = [sys.executable, str(SCRIPT), "--source", str(self.source), "--rclone", RCLONE]
        result = subprocess.run(command, env=environment, capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.segment.exists())
        self.assertEqual(gzip.decompress((self.target / (self.segment.name + ".gz")).read_bytes()), self.body)

    def test_multichunk_compression_and_remote_readback_preserve_bytes(self):
        body = os.urandom(2 * 1024 * 1024 + 17)
        self.segment.write_bytes(body)
        result = self.run_backup()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(gzip.decompress((self.target / (self.segment.name + ".gz")).read_bytes()), body)

    def test_mount_disappearing_after_upload_prevents_local_cleanup(self):
        spec = importlib.util.spec_from_file_location("audit_backup", SCRIPT)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        args = SimpleNamespace(source=str(self.source), destination=str(self.target),
                               destination_mount=str(self.root), allow_local=False,
                               rclone=RCLONE, command_timeout=30, keep_source=False)
        with mock.patch.object(module.os.path, "ismount", side_effect=[True, True, False]), \
                mock.patch.dict(os.environ, self.environment), mock.patch.object(module, "report"):
            self.assertEqual(module.run(args), 1)
        self.assertTrue(self.segment.exists())
        self.assertTrue((self.target / (self.segment.name + ".gz")).exists())

    def test_upload_failure_keeps_source_and_hides_tool_output(self):
        wrapper = self.wrapper("if 'copyto' in sys.argv:\n print('private-tool-output')\n sys.stderr.write('private-tool-error')\n sys.exit(7)\n")
        result = self.run_backup(wrapper)
        self.assert_preserved(result)
        self.assertNotIn("private-tool", result.stdout + result.stderr)
        self.assertEqual(self.segment.read_bytes(), self.body)
        retried = self.run_backup()
        self.assertEqual(retried.returncode, 0, retried.stdout + retried.stderr)
        self.assertFalse(self.segment.exists())

    def test_readback_failure_keeps_source_even_after_copy(self):
        wrapper = self.wrapper("if 'cat' in sys.argv: sys.exit(7)\n")
        self.assert_preserved(self.run_backup(wrapper))
        self.assertTrue((self.target / (self.segment.name + ".gz")).exists())
        self.assertEqual(self.run_backup().returncode, 0)

    def test_readback_content_mismatch_keeps_source(self):
        wrapper = self.wrapper("if 'cat' in sys.argv:\n sys.stdout.buffer.write(b'wrong bytes')\n sys.exit(0)\n")
        self.assert_preserved(self.run_backup(wrapper))

    def test_readback_has_a_parent_enforced_timeout(self):
        wrapper = self.wrapper("if 'copyto' in sys.argv:\n os.makedirs(os.path.dirname(sys.argv[-1]), exist_ok=True)\n shutil.copyfile(sys.argv[-2], sys.argv[-1])\n sys.exit(0)\nif 'cat' in sys.argv: time.sleep(30)\n")
        started = time.monotonic()
        self.assert_preserved(self.run_backup(wrapper, "--command-timeout", "1"))
        self.assertLess(time.monotonic() - started, 5)
        self.assertTrue((self.target / (self.segment.name + ".gz")).exists())

    def test_upload_has_a_parent_enforced_timeout(self):
        wrapper = self.wrapper("if 'copyto' in sys.argv: time.sleep(30)\n")
        started = time.monotonic()
        self.assert_preserved(self.run_backup(wrapper, "--command-timeout", "1"))
        self.assertLess(time.monotonic() - started, 5)
        self.assertEqual(self.segment.read_bytes(), self.body)
        self.assertEqual(self.run_backup().returncode, 0)
        self.assertFalse(self.segment.exists())

    def test_existing_different_remote_object_is_not_overwritten(self):
        result = self.run_backup(None, "--keep-source")
        self.assertEqual(result.returncode, 0, result.stdout)
        remote = self.target / (self.segment.name + ".gz")
        data = bytearray(remote.read_bytes())
        data[-1] ^= 1  # Same length; a size-only comparison would miss this.
        remote.write_bytes(data)
        result = self.run_backup()
        self.assert_preserved(result)
        self.assertEqual(remote.read_bytes(), data)

    def test_source_changed_during_verification_is_not_removed(self):
        wrapper = self.wrapper("if 'cat' in sys.argv:\n open(" + repr(str(self.segment)) + ", 'wb').write(b'changed source\\n')\n")
        self.assert_preserved(self.run_backup(wrapper))
        self.assertEqual(self.segment.read_bytes(), b"changed source\n")

    def test_symlink_source_is_not_followed(self):
        outside = self.root / "private-file"
        outside.write_bytes(b"do not upload")
        self.segment.unlink()
        self.segment.symlink_to(outside)
        self.assert_preserved(self.run_backup())
        self.assertEqual(outside.read_bytes(), b"do not upload")
        self.assertFalse(self.target.exists())

    def test_second_job_skips_while_lock_is_held(self):
        with (self.source / ".backup.lock").open("a") as lock:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.run_backup()
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertIn('"event": "skipped"', result.stdout)
        self.assertTrue(self.segment.exists())
        self.assertFalse(self.target.exists())

    def test_unmounted_destination_and_unconfigured_target_fail_closed(self):
        command = self.command()
        command.remove("--allow-local")
        result = subprocess.run(command, env=self.environment, capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 2, result.stdout)
        command += ["--destination-mount", str(self.root / "missing-mount")]
        result = subprocess.run(command, env=self.environment, capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 2, result.stdout)
        result = self.run_backup(None, "--destination", "")
        self.assertEqual(result.returncode, 2, result.stdout)
        self.assertTrue(self.segment.exists())
        self.assertFalse(self.target.exists())

    def test_crash_during_upload_is_retried_without_losing_source(self):
        marker = self.root / "upload-started"
        wrapper = self.wrapper("if 'copyto' in sys.argv:\n open(" + repr(str(marker)) + ", 'w').close()\n time.sleep(30)\n")
        process = subprocess.Popen(self.command(wrapper), env=self.environment,
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        try:
            deadline = time.monotonic() + 5
            while not marker.exists():
                self.assertLess(time.monotonic(), deadline, "upload helper did not start")
                time.sleep(0.01)
        finally:
            os.killpg(process.pid, signal.SIGTERM)
            process.wait(timeout=5)
        self.assertTrue(self.segment.exists())
        result = self.run_backup()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.segment.exists())
        self.assertEqual(list(self.source.glob(".backup-*.gz")), [])

    def test_one_failed_file_does_not_prevent_other_backups(self):
        broken = self.source / "audit-a-broken.log"
        broken.symlink_to(self.root / "missing")
        result = self.run_backup()
        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertFalse(self.segment.exists())
        self.assertTrue(broken.is_symlink())
        self.assertEqual(gzip.decompress((self.target / (self.segment.name + ".gz")).read_bytes()), self.body)


if __name__ == "__main__":
    unittest.main()
