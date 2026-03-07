from __future__ import annotations

import json
import sys

from .exporters import export_table_to_parquet, test_connection
from .models import Backend, ExportRequest


def _emit(event_type: str, payload: object) -> None:
    print(json.dumps({"type": event_type, "payload": payload}, ensure_ascii=False), flush=True)


def _log(message: str) -> None:
    _emit("log", message)


def _load_request() -> ExportRequest:
    payload = json.load(sys.stdin)
    return ExportRequest(
        backend=Backend(payload["backend"]),
        output_path=payload.get("outputPath", ""),
        table=payload.get("table", ""),
        batch_size=int(payload.get("batchSize", 5000) or 5000),
        compression=payload.get("compression", "snappy"),
        schema=payload.get("schema", ""),
        host=payload.get("host", ""),
        port=int(payload["port"]) if payload.get("port") not in (None, "", 0) else None,
        username=payload.get("username", ""),
        password=payload.get("password", ""),
        database=payload.get("database", ""),
        maxcompute_endpoint=payload.get("maxcomputeEndpoint", ""),
        maxcompute_project=payload.get("maxcomputeProject", ""),
        maxcompute_access_id=payload.get("maxcomputeAccessId", ""),
        maxcompute_access_key=payload.get("maxcomputeAccessKey", ""),
        partition_spec=payload.get("partitionSpec", ""),
    )


def main() -> int:
    if len(sys.argv) != 2 or sys.argv[1] not in {"test", "export"}:
        print("usage: python -m parquet_export_gui.bridge [test|export]", file=sys.stderr)
        return 2

    kind = sys.argv[1]
    request = _load_request()

    try:
        if kind == "test":
            test_connection(request, on_log=_log)
            _emit("success", {"output_path": "", "rows_written": 0})
        else:
            result = export_table_to_parquet(request, on_log=_log)
            _emit(
                "success",
                {
                    "output_path": str(result.output_path),
                    "rows_written": result.rows_written,
                },
            )
    except Exception as exc:
        _emit("error", str(exc))
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
