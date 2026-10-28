#!/usr/bin/env python3
"""Archive immutable Done Hub audit segments through rclone, then remove them.

Requires Python 3.9+ and rclone. No Done Hub API or credentials are used.
"""

import argparse
import fcntl
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import threading


def report(event, **fields):
    print(json.dumps({"event": event, **fields}, ensure_ascii=False), flush=True)


def identity(info):
    return (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)


def check_destination(args):
    # Prefer a named rclone remote; credentials stay in rclone's own config.
    if re.match(r"^[\w .-]+:", args.destination):
        if args.destination_mount:
            raise ValueError("destination-mount only applies to local paths")
        return
    destination = Path(args.destination)
    if not destination.is_absolute():
        raise ValueError("destination must be a named rclone remote or an absolute path")
    destination = destination.resolve()
    if destination == args.source or args.source in destination.parents:
        raise ValueError("destination must be outside the audit archive directory")
    if args.destination_mount:
        mount = Path(args.destination_mount).resolve()
        if not os.path.ismount(mount) or (destination != mount and mount not in destination.parents):
            raise ValueError("configured destination mount is unavailable or does not contain destination")
    elif not args.allow_local:
        raise ValueError("local destination requires --destination-mount or explicit --allow-local")


def rclone_args(args):
    return [args.rclone, "--retries", "1", "--low-level-retries", "2",
            "--contimeout", "10s", "--timeout", "30s", "--max-duration", "5m", "--log-level", "ERROR"]


def backup_file(args, source):
    before = source.lstat()
    if not stat.S_ISREG(before.st_mode):
        raise ValueError("sealed segment is not a regular file")
    # A deterministic gzip stream makes retrying an already uploaded segment
    # idempotent. Only one compressed temporary file exists during a run.
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(dir=args.source, prefix=".backup-", suffix=".gz", delete=False) as output:
            temporary = Path(output.name)
            descriptor = os.open(source, os.O_RDONLY | os.O_NOFOLLOW)
            with os.fdopen(descriptor, "rb") as incoming:
                if identity(os.fstat(incoming.fileno())) != identity(before):
                    raise ValueError("sealed segment changed before compression")
                with gzip.GzipFile(filename="", mode="wb", fileobj=output, mtime=0) as compressed:
                    shutil.copyfileobj(incoming, compressed, length=1024 * 1024)
                if identity(os.fstat(incoming.fileno())) != identity(before):
                    raise ValueError("sealed segment changed during compression")
        with temporary.open("rb") as compressed:
            expected = hashlib.sha256()
            for block in iter(lambda: compressed.read(1024 * 1024), b""):
                expected.update(block)
        remote = args.destination.rstrip("/") + "/" + source.name + ".gz"
        check_destination(args)
        copied = subprocess.run(rclone_args(args) + ["copyto", "--ignore-existing", str(temporary), remote],
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=args.command_timeout)
        if copied.returncode:
            raise RuntimeError("rclone copyto failed (exit %d); source retained" % copied.returncode)
        # Download the actual remote bytes. This also works with WebDAV and
        # crypt remotes without assuming they support server-side hashes.
        actual = hashlib.sha256()
        with subprocess.Popen(rclone_args(args) + ["cat", remote], stdout=subprocess.PIPE,
                              stderr=subprocess.DEVNULL) as reader:
            deadline = threading.Timer(args.command_timeout, reader.kill)
            deadline.daemon = True
            deadline.start()
            try:
                for block in iter(lambda: reader.stdout.read(1024 * 1024), b""):
                    actual.update(block)
                status = reader.wait()
            finally:
                deadline.cancel()
        if status or actual.digest() != expected.digest():
            raise RuntimeError("remote readback failed or content mismatch; source retained")
        check_destination(args)
        if identity(source.lstat()) != identity(before):
            raise ValueError("sealed segment changed before cleanup; source retained")
        if not args.keep_source:
            source.unlink()
        report("verified", file=source.name, bytes=before.st_size,
               sha256=expected.hexdigest(), source_removed=not args.keep_source)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def run(args):
    args.source = Path(args.source).resolve()
    if not args.destination:
        raise ValueError("set AUDIT_BACKUP_DEST or --destination")
    if args.command_timeout <= 0:
        raise ValueError("command-timeout must be positive")
    if not shutil.which(args.rclone):
        raise ValueError("rclone executable not found")
    check_destination(args)
    if not args.source.exists():
        report("idle", reason="archive directory does not exist yet")
        return 0
    with (args.source / ".backup.lock").open("a") as lock:
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            report("skipped", reason="another backup job is running")
            return 0
        # Leftovers from an interrupted job are only compressed copies; the
        # original .log remains until remote verification has succeeded.
        for temporary in args.source.glob(".backup-*.gz"):
            if temporary.is_file() and not temporary.is_symlink():
                temporary.unlink()
        failed = 0
        for source in sorted(args.source.glob("audit-*.log")):
            try:
                backup_file(args, source)
            except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
                failed += 1
                report("failed", file=source.name, error=str(error))
        remaining = [p for p in args.source.glob("audit-*.log") if p.is_file() and not p.is_symlink()]
        report("summary", failed=failed, pending_files=len(remaining),
               pending_bytes=sum(p.stat().st_size for p in remaining))
        return 1 if failed else 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", default=os.getenv("AUDIT_ARCHIVE_DIR", "/home/donehub/data/logs/audit/archive"))
    parser.add_argument("--destination", default=os.getenv("AUDIT_BACKUP_DEST", ""))
    parser.add_argument("--destination-mount", default=os.getenv("AUDIT_BACKUP_MOUNT", ""))
    parser.add_argument("--allow-local", action="store_true", help="explicitly allow an unmounted local destination (testing)")
    parser.add_argument("--keep-source", action="store_true", help="verify upload without removing the source (first-run check)")
    parser.add_argument("--rclone", default="rclone")
    parser.add_argument("--command-timeout", type=float, default=330, help="wall-clock timeout in seconds for each rclone command")
    args = parser.parse_args()
    os.umask(0o077)
    try:
        return run(args)
    except (OSError, ValueError) as error:
        report("configuration_error", error=str(error))
        return 2


if __name__ == "__main__":
    sys.exit(main())
