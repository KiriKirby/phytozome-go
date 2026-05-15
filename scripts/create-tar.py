#!/usr/bin/env python3
import os
import stat
import sys
import tarfile
from pathlib import Path


def main() -> int:
    if len(sys.argv) < 4:
        print("usage: create-tar.py <archive> <source_root> <root_name> [exec_prefix...]", file=sys.stderr)
        return 2

    archive_path = Path(sys.argv[1])
    source_root = Path(sys.argv[2])
    root_name = sys.argv[3].replace("\\", "/").rstrip("/")
    exec_prefixes = [prefix.replace("\\", "/").rstrip("/") for prefix in sys.argv[4:]]

    with tarfile.open(archive_path, "w:gz") as tar:
        root_info = tarfile.TarInfo(root_name)
        root_info.type = tarfile.DIRTYPE
        root_info.mode = 0o755
        root_info.mtime = int(source_root.stat().st_mtime)
        tar.addfile(root_info)

        for path in sorted(source_root.rglob("*")):
            rel = path.relative_to(source_root).as_posix()
            arcname = f"{root_name}/{rel}"
            info = tarfile.TarInfo(arcname)
            st = path.lstat()
            info.mtime = int(st.st_mtime)
            if path.is_dir():
                info.type = tarfile.DIRTYPE
                info.mode = 0o755
                info.size = 0
                tar.addfile(info)
                continue

            info.size = st.st_size
            if any(rel == prefix or rel.startswith(prefix + "/") for prefix in exec_prefixes):
                info.mode = 0o755
            elif path.suffix in {".sh", ".app"}:
                info.mode = 0o755
            else:
                info.mode = 0o644

            with path.open("rb") as fh:
                tar.addfile(info, fh)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
