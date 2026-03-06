from __future__ import annotations

from contextlib import closing
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, Iterable

import pyarrow as pa
import pyarrow.compute as pc
import pyarrow.parquet as pq

from .models import Backend, DEFAULT_PORTS, ExportRequest

LogCallback = Callable[[str], None]


@dataclass(slots=True)
class ExportResult:
    output_path: Path
    rows_written: int


def test_connection(request: ExportRequest, on_log: LogCallback | None = None) -> None:
    log = on_log or _noop
    _validate_request(request, require_output=False, require_table=False)
    log(f"测试连接: {request.backend.value}")
    if request.is_sql_backend:
        _test_sql_connection(request)
    else:
        _test_maxcompute_connection(request)
    log("连接成功")


def export_table_to_parquet(
    request: ExportRequest,
    on_log: LogCallback | None = None,
) -> ExportResult:
    log = on_log or _noop
    _validate_request(request, require_output=True)
    output_path = Path(request.output_path).expanduser().resolve()
    output_path.parent.mkdir(parents=True, exist_ok=True)
    log(f"准备导出 {request.full_table_name} -> {output_path}")
    if request.is_sql_backend:
        rows_written = _export_sql_table(request, output_path, log)
    else:
        rows_written = _export_maxcompute_table(request, output_path, log)
    log(f"导出完成，总计 {rows_written:,} 行")
    return ExportResult(output_path=output_path, rows_written=rows_written)


def _validate_request(
    request: ExportRequest,
    *,
    require_output: bool,
    require_table: bool = True,
) -> None:
    if require_table and not request.table.strip():
        raise ValueError("表名不能为空")
    if request.batch_size <= 0:
        raise ValueError("批次大小必须大于 0")
    if require_output and not request.output_path.strip():
        raise ValueError("输出路径不能为空")
    if request.compression not in {"snappy", "gzip", "brotli", "lz4", "zstd", "none"}:
        raise ValueError("不支持的压缩格式")
    if request.is_sql_backend:
        missing = []
        if not request.host.strip():
            missing.append("Host")
        if not request.username.strip():
            missing.append("Username")
        if not request.database.strip():
            missing.append("Database / Service Name")
        if missing:
            raise ValueError(f"缺少必填项: {', '.join(missing)}")
        return
    missing = []
    if not request.maxcompute_endpoint.strip():
        missing.append("Endpoint")
    if not request.maxcompute_project.strip():
        missing.append("Project")
    if not request.maxcompute_access_id.strip():
        missing.append("Access ID")
    if not request.maxcompute_access_key.strip():
        missing.append("Access Key")
    if missing:
        raise ValueError(f"缺少必填项: {', '.join(missing)}")


def _test_sql_connection(request: ExportRequest) -> None:
    with closing(_open_sql_connection(request)) as connection:
        with closing(connection.cursor()) as cursor:
            sql = "SELECT 1 FROM DUAL" if request.backend is Backend.ORACLE else "SELECT 1"
            cursor.execute(sql)
            cursor.fetchone()


def _test_maxcompute_connection(request: ExportRequest) -> None:
    odps = _build_odps_client(request)
    project = odps.get_project()
    project.reload()


def _export_sql_table(request: ExportRequest, output_path: Path, log: LogCallback) -> int:
    compression = None if request.compression == "none" else request.compression
    writer: pq.ParquetWriter | None = None
    rows_written = 0
    select_sql = f"SELECT * FROM {request.full_table_name}"
    log(f"执行查询: {select_sql}")
    try:
        with closing(_open_sql_connection(request)) as connection:
            with closing(_open_sql_cursor(connection, request)) as cursor:
                cursor.execute(select_sql)
                columns = [desc[0] for desc in cursor.description or []]
                while True:
                    rows = cursor.fetchmany(request.batch_size)
                    if not rows:
                        break
                    pylist = [_row_to_mapping(columns, row) for row in rows]
                    table = pa.Table.from_pylist(pylist)
                    if writer is None:
                        writer = pq.ParquetWriter(output_path, table.schema, compression=compression)
                    else:
                        table = _align_table(table, writer.schema)
                    writer.write_table(table)
                    rows_written += table.num_rows
                    log(f"已写入 {rows_written:,} 行")
                if writer is None:
                    empty_table = _empty_table(columns)
                    writer = pq.ParquetWriter(output_path, empty_table.schema, compression=compression)
                    writer.write_table(empty_table)
                    log("源表为空，已生成空的 Parquet 文件")
        return rows_written
    finally:
        if writer is not None:
            writer.close()


def _export_maxcompute_table(request: ExportRequest, output_path: Path, log: LogCallback) -> int:
    from odps import options

    compression = None if request.compression == "none" else request.compression
    odps = _build_odps_client(request)
    table = odps.get_table(request.table, schema=request.schema or None)
    column_names = _maxcompute_column_names(table)
    writer: pq.ParquetWriter | None = None
    rows_written = 0
    options.tunnel.use_instance_tunnel = True
    log(f"读取 MaxCompute 表: {request.full_table_name}")
    try:
        with table.open_reader(
            partition=request.partition_spec or None,
            arrow=True,
            append_partitions=bool(request.partition_spec),
        ) as reader:
            for batch in reader:
                if batch.num_rows == 0:
                    continue
                arrow_table = pa.Table.from_batches([batch])
                if writer is None:
                    writer = pq.ParquetWriter(output_path, arrow_table.schema, compression=compression)
                else:
                    arrow_table = _align_table(arrow_table, writer.schema)
                writer.write_table(arrow_table)
                rows_written += batch.num_rows
                log(f"已写入 {rows_written:,} 行")
            if writer is None:
                empty_table = _empty_table(column_names)
                writer = pq.ParquetWriter(output_path, empty_table.schema, compression=compression)
                writer.write_table(empty_table)
                log("源表为空，已生成空的 Parquet 文件")
        return rows_written
    finally:
        if writer is not None:
            writer.close()


def _open_sql_connection(request: ExportRequest) -> Any:
    if request.backend is Backend.ORACLE:
        import oracledb

        return oracledb.connect(
            user=request.username,
            password=request.password,
            dsn=f"{request.host}:{request.port or DEFAULT_PORTS[Backend.ORACLE]}/{request.database}",
        )
    if request.backend is Backend.MYSQL:
        import pymysql

        return pymysql.connect(
            host=request.host,
            port=request.port or DEFAULT_PORTS[Backend.MYSQL],
            user=request.username,
            password=request.password,
            database=request.database,
            charset="utf8mb4",
            autocommit=True,
            cursorclass=pymysql.cursors.SSCursor,
        )
    if request.backend is Backend.POSTGRESQL:
        import psycopg

        return psycopg.connect(
            host=request.host,
            port=request.port or DEFAULT_PORTS[Backend.POSTGRESQL],
            user=request.username,
            password=request.password,
            dbname=request.database,
            autocommit=False,
        )
    raise ValueError(f"不支持的 SQL 后端: {request.backend}")


def _open_sql_cursor(connection: Any, request: ExportRequest) -> Any:
    if request.backend is Backend.POSTGRESQL:
        cursor = connection.cursor(name="parquet_export_stream")
        cursor.itersize = request.batch_size
        return cursor
    cursor = connection.cursor()
    if request.backend is Backend.ORACLE:
        cursor.arraysize = request.batch_size
    return cursor


def _build_odps_client(request: ExportRequest) -> Any:
    from odps import ODPS

    return ODPS(
        access_id=request.maxcompute_access_id,
        secret_access_key=request.maxcompute_access_key,
        project=request.maxcompute_project,
        endpoint=request.maxcompute_endpoint,
    )


def _row_to_mapping(columns: list[str], row: Iterable[Any]) -> dict[str, Any]:
    return {
        column: _normalize_value(value)
        for column, value in zip(columns, row, strict=False)
    }


def _normalize_value(value: Any) -> Any:
    if value is None:
        return None
    read_method = getattr(value, "read", None)
    if callable(read_method):
        try:
            return read_method()
        except TypeError:
            pass
    if isinstance(value, memoryview):
        return value.tobytes()
    if isinstance(value, bytearray):
        return bytes(value)
    return value


def _align_table(table: pa.Table, target_schema: pa.Schema) -> pa.Table:
    if table.schema == target_schema:
        return table
    arrays = []
    for field in target_schema:
        if field.name in table.schema.names:
            column = table[field.name]
            if column.type != field.type:
                column = pc.cast(column, target_type=field.type, safe=False)
        else:
            column = pa.nulls(table.num_rows, type=field.type)
        arrays.append(column)
    return pa.Table.from_arrays(arrays, schema=target_schema)


def _empty_table(column_names: list[str]) -> pa.Table:
    return pa.Table.from_arrays(
        [pa.array([], type=pa.null()) for _ in column_names],
        names=column_names,
    )


def _maxcompute_column_names(table: Any) -> list[str]:
    schema = getattr(table, "table_schema", None)
    if schema is None:
        return []
    names = getattr(schema, "names", None)
    if names:
        return list(names)
    columns = getattr(schema, "simple_columns", None) or getattr(schema, "columns", None) or []
    return [column.name for column in columns]


def _noop(_: str) -> None:
    return None
