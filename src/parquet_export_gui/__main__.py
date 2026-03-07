from __future__ import annotations

import multiprocessing
import sys

try:
    from .main import main
except ImportError:
    from parquet_export_gui.main import main


def _handle_multiprocessing_subprocess() -> bool:
    multiprocessing.freeze_support()

    if "-c" not in sys.argv:
        return False

    code_index = sys.argv.index("-c") + 1
    if code_index >= len(sys.argv):
        return False

    code = sys.argv[code_index]
    prefix = "from multiprocessing.resource_tracker import main;main("
    if not code.startswith(prefix) or not code.endswith(")"):
        return False

    from multiprocessing.resource_tracker import main as resource_tracker_main

    resource_tracker_main(int(code[len(prefix):-1]))
    return True


if __name__ == "__main__":
    if not _handle_multiprocessing_subprocess():
        main()
