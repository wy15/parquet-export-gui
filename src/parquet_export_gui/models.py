from __future__ import annotations

from dataclasses import dataclass
from enum import Enum


class Backend(str, Enum):
    ORACLE = "oracle"
    MYSQL = "mysql"
    POSTGRESQL = "postgresql"
    MAXCOMPUTE = "maxcompute"


DEFAULT_PORTS: dict[Backend, int] = {
    Backend.ORACLE: 1521,
    Backend.MYSQL: 3306,
    Backend.POSTGRESQL: 5432,
}


COMPRESSION_OPTIONS = ["snappy", "gzip", "brotli", "lz4", "zstd", "none"]


@dataclass(slots=True)
class ExportRequest:
    backend: Backend
    output_path: str
    table: str
    batch_size: int = 5000
    compression: str = "snappy"
    schema: str = ""
    host: str = ""
    port: int | None = None
    username: str = ""
    password: str = ""
    database: str = ""
    maxcompute_endpoint: str = ""
    maxcompute_project: str = ""
    maxcompute_access_id: str = ""
    maxcompute_access_key: str = ""
    partition_spec: str = ""

    @property
    def is_sql_backend(self) -> bool:
        return self.backend in {Backend.ORACLE, Backend.MYSQL, Backend.POSTGRESQL}

    @property
    def full_table_name(self) -> str:
        return f"{self.schema}.{self.table}" if self.schema else self.table


def backend_label(backend: Backend) -> str:
    labels = {
        Backend.ORACLE: "Oracle (Thin)",
        Backend.MYSQL: "MySQL",
        Backend.POSTGRESQL: "PostgreSQL",
        Backend.MAXCOMPUTE: "MaxCompute",
    }
    return labels[backend]


def database_field_label(backend: Backend) -> str:
    if backend is Backend.ORACLE:
        return "Service Name"
    if backend is Backend.MAXCOMPUTE:
        return "Project"
    return "Database"
