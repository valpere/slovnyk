#!/usr/bin/env python3
"""Provision a durable local artifact root before dataset verification."""
import argparse
import os
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    root = args.directory.absolute()
    root.mkdir(parents=True, exist_ok=True, mode=0o750)
    # Persist the directory entries as well as the directory itself. Installers
    # fsync descendants; provisioning owns this root's ancestor chain.
    for directory in [root, *root.parents]:
        fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    print(root)


if __name__ == "__main__":
    main()
