from __future__ import annotations

import queue
import threading
from datetime import datetime
from pathlib import Path
from typing import Any

from nicegui import app, ui

from .exporters import ExportResult, export_table_to_parquet, test_connection
from .models import COMPRESSION_OPTIONS, DEFAULT_PORTS, Backend, ExportRequest, backend_label

app.native.window_args["resizable"] = True
app.native.window_args["min_size"] = (1100, 780)
app.native.settings["ALLOW_DOWNLOADS"] = True


def run() -> None:
    ui.colors(
        primary="#0f766e",
        secondary="#164e63",
        accent="#ea580c",
        positive="#047857",
        negative="#b91c1c",
        warning="#b45309",
    )
    ui.add_head_html(
        """
        <style>
            body {
                font-family: "Avenir Next", "Segoe UI", "PingFang SC", "Helvetica Neue", sans-serif;
                background:
                    radial-gradient(circle at top left, rgba(15,118,110,0.18), transparent 24%),
                    radial-gradient(circle at bottom right, rgba(234,88,12,0.14), transparent 22%),
                    linear-gradient(180deg, #f4fbfa 0%, #eef6f7 100%);
            }
            .glass-card {
                background: rgba(255, 255, 255, 0.88);
                backdrop-filter: blur(10px);
                border: 1px solid rgba(15, 23, 42, 0.06);
                box-shadow: 0 22px 60px rgba(15, 23, 42, 0.08);
                border-radius: 24px;
            }
            .hero-title {
                letter-spacing: -0.04em;
            }
            .mono-note {
                font-family: "SF Mono", "JetBrains Mono", "Menlo", monospace;
            }
        </style>
        """
    )

    state: dict[str, Any] = {
        "backend": Backend.ORACLE.value,
        "host": "",
        "port": DEFAULT_PORTS[Backend.ORACLE],
        "username": "",
        "password": "",
        "database": "",
        "schema": "",
        "table": "",
        "output_path": str(Path.home() / "Downloads" / "export.parquet"),
        "batch_size": 5000,
        "compression": "snappy",
        "maxcompute_endpoint": "",
        "maxcompute_project": "",
        "maxcompute_access_id": "",
        "maxcompute_access_key": "",
        "partition_spec": "",
        "running": False,
        "logs": [],
        "rows_written": 0,
        "status": "待命",
    }
    event_queue: queue.Queue[tuple[str, Any]] = queue.Queue()

    def set_state(key: str, value: Any) -> None:
        state[key] = value

    def append_log(message: str) -> None:
        event_queue.put(("log", message))

    def set_running(is_running: bool) -> None:
        state["running"] = is_running
        if is_running:
            export_button.disable()
            test_button.disable()
        else:
            export_button.enable()
            test_button.enable()

    def set_status(text: str) -> None:
        state["status"] = text
        status_label.text = text
        status_label.update()

    def backend_changed(value: str) -> None:
        backend = Backend(value)
        state["backend"] = value
        if backend in DEFAULT_PORTS:
            port_input.value = DEFAULT_PORTS[backend]
            port_input.update()
            state["port"] = DEFAULT_PORTS[backend]
        sql_panel.style(f"display: {'block' if backend is not Backend.MAXCOMPUTE else 'none'};")
        maxcompute_panel.style(f"display: {'block' if backend is Backend.MAXCOMPUTE else 'none'};")
        partition_row.style(f"display: {'flex' if backend is Backend.MAXCOMPUTE else 'none'};")
        backend_hint.text = _backend_hint(backend)
        backend_hint.update()

    def build_request() -> ExportRequest:
        backend = Backend(state["backend"])
        port_value = state["port"]
        port = int(port_value) if port_value not in ("", None) else None
        return ExportRequest(
            backend=backend,
            host=str(state["host"]).strip(),
            port=port,
            username=str(state["username"]).strip(),
            password=str(state["password"]),
            database=str(state["database"]).strip(),
            schema=str(state["schema"]).strip(),
            table=str(state["table"]).strip(),
            output_path=str(state["output_path"]).strip(),
            batch_size=max(int(state["batch_size"]), 1),
            compression=str(state["compression"]),
            maxcompute_endpoint=str(state["maxcompute_endpoint"]).strip(),
            maxcompute_project=str(state["maxcompute_project"]).strip(),
            maxcompute_access_id=str(state["maxcompute_access_id"]).strip(),
            maxcompute_access_key=str(state["maxcompute_access_key"]).strip(),
            partition_spec=str(state["partition_spec"]).strip(),
        )

    def run_in_thread(kind: str) -> None:
        if state["running"]:
            return
        try:
            request = build_request()
        except Exception as exc:
            ui.notify(str(exc), type="negative")
            return

        state["rows_written"] = 0
        set_running(True)
        set_status("运行中")
        rows_label.text = "已写入 0 行"
        rows_label.update()
        append_log(f"开始{ '导出' if kind == 'export' else '测试连接' }")

        def worker() -> None:
            try:
                if kind == "export":
                    result = export_table_to_parquet(request, on_log=append_log)
                    event_queue.put(("success", result))
                else:
                    test_connection(request, on_log=append_log)
                    event_queue.put(("tested", None))
            except Exception as exc:
                event_queue.put(("error", str(exc)))
            finally:
                event_queue.put(("running", False))

        threading.Thread(target=worker, daemon=True).start()

    def process_events() -> None:
        dirty_log = False
        while not event_queue.empty():
            kind, payload = event_queue.get_nowait()
            if kind == "log":
                timestamp = datetime.now().strftime("%H:%M:%S")
                state["logs"].append(f"[{timestamp}] {payload}")
                state["logs"] = state["logs"][-300:]
                dirty_log = True
            elif kind == "success":
                result = payload
                assert isinstance(result, ExportResult)
                state["rows_written"] = result.rows_written
                rows_label.text = f"已写入 {result.rows_written:,} 行"
                rows_label.update()
                set_status("导出完成")
                ui.notify(f"导出成功: {result.output_path}", type="positive", timeout=8000)
            elif kind == "tested":
                set_status("连接成功")
                ui.notify("连接成功", type="positive")
            elif kind == "error":
                set_status("执行失败")
                state["logs"].append(f"[{datetime.now().strftime('%H:%M:%S')}] ERROR: {payload}")
                dirty_log = True
                ui.notify(str(payload), type="negative", timeout=8000)
            elif kind == "running":
                set_running(bool(payload))
                if not payload and state["status"] == "运行中":
                    set_status("待命")
        if dirty_log:
            log_view.value = "\n".join(state["logs"])
            log_view.update()

    with ui.column().classes("w-full items-center px-6 py-8 gap-6"):
        with ui.card().classes("glass-card w-full max-w-[1320px] p-8 gap-6"):
            with ui.row().classes("w-full items-center justify-between"):
                with ui.column().classes("gap-2"):
                    ui.label("Parquet Export Studio").classes("hero-title text-[34px] font-bold text-slate-900")
                    ui.label("从 Oracle / MySQL / PostgreSQL / MaxCompute 导出单表到本地 Parquet")
                    backend_hint = ui.label(_backend_hint(Backend.ORACLE)).classes("text-sm text-teal-700")
                with ui.column().classes("items-end gap-2"):
                    status_label = ui.label("待命").classes("text-sm font-semibold text-teal-700")
                    rows_label = ui.label("已写入 0 行").classes("mono-note text-sm text-slate-600")

            with ui.row().classes("w-full gap-6 items-start"):
                with ui.card().classes("glass-card w-[430px] shrink-0 p-6 gap-4"):
                    ui.label("连接配置").classes("text-lg font-semibold text-slate-900")
                    ui.select(
                        {item.value: backend_label(item) for item in Backend},
                        value=state["backend"],
                        label="数据源",
                        on_change=lambda event: backend_changed(event.value),
                    ).props("outlined")

                    sql_panel = ui.column().classes("w-full gap-4")
                    with sql_panel:
                        ui.input(
                            "Host",
                            value=state["host"],
                            on_change=lambda event: set_state("host", event.value),
                        ).props("outlined")
                        port_input = ui.input(
                            "Port",
                            value=str(state["port"]),
                            on_change=lambda event: set_state("port", event.value),
                        ).props("outlined")
                    with sql_panel:
                        with ui.row().classes("w-full gap-3"):
                            ui.input(
                                "Username",
                                value=state["username"],
                                on_change=lambda event: set_state("username", event.value),
                            ).props("outlined").classes("flex-1")
                            ui.input(
                                "Password",
                                password=True,
                                password_toggle_button=True,
                                value=state["password"],
                                on_change=lambda event: set_state("password", event.value),
                            ).props("outlined").classes("flex-1")
                        ui.input(
                            "Database / Service Name",
                            value=state["database"],
                            on_change=lambda event: set_state("database", event.value),
                        ).props("outlined")

                    maxcompute_panel = ui.column().classes("w-full gap-4")
                    with maxcompute_panel:
                        ui.input(
                            "Endpoint",
                            value=state["maxcompute_endpoint"],
                            on_change=lambda event: set_state("maxcompute_endpoint", event.value),
                        ).props("outlined")
                        ui.input(
                            "Project",
                            value=state["maxcompute_project"],
                            on_change=lambda event: set_state("maxcompute_project", event.value),
                        ).props("outlined")
                        ui.input(
                            "Access ID",
                            value=state["maxcompute_access_id"],
                            on_change=lambda event: set_state("maxcompute_access_id", event.value),
                        ).props("outlined")
                        ui.input(
                            "Access Key",
                            password=True,
                            password_toggle_button=True,
                            value=state["maxcompute_access_key"],
                            on_change=lambda event: set_state("maxcompute_access_key", event.value),
                        ).props("outlined")

                    with ui.row().classes("w-full gap-3"):
                        ui.input(
                            "Schema (optional)",
                            value=state["schema"],
                            on_change=lambda event: set_state("schema", event.value),
                        ).props("outlined").classes("flex-1")
                        ui.input(
                            "Table",
                            value=state["table"],
                            on_change=lambda event: set_state("table", event.value),
                        ).props("outlined").classes("flex-1")

                    partition_row = ui.row().classes("w-full gap-3")
                    with partition_row:
                        ui.input(
                            "Partition Spec",
                            placeholder="ds=20260306,region=cn",
                            value=state["partition_spec"],
                            on_change=lambda event: set_state("partition_spec", event.value),
                        ).props("outlined").classes("w-full")

                with ui.column().classes("flex-1 gap-6"):
                    with ui.card().classes("glass-card w-full p-6 gap-4"):
                        ui.label("导出参数").classes("text-lg font-semibold text-slate-900")
                        ui.input(
                            "输出 Parquet 路径",
                            value=state["output_path"],
                            on_change=lambda event: set_state("output_path", event.value),
                        ).props("outlined").classes("w-full")
                        with ui.row().classes("w-full gap-3"):
                            ui.number(
                                "批次大小",
                                value=state["batch_size"],
                                min=1,
                                step=1000,
                                on_change=lambda event: set_state("batch_size", int(event.value or 1)),
                            ).props("outlined").classes("flex-1")
                            ui.select(
                                COMPRESSION_OPTIONS,
                                value=state["compression"],
                                label="压缩",
                                on_change=lambda event: set_state("compression", event.value),
                            ).props("outlined").classes("flex-1")
                        ui.label(
                            "建议把输出路径指向本地磁盘。大表可适当调大批次，网络不稳定时建议保守设置。"
                        ).classes("text-sm text-slate-600")
                        with ui.row().classes("w-full gap-3"):
                            test_button = ui.button(
                                "测试连接",
                                on_click=lambda: run_in_thread("test"),
                            ).props("unelevated color=secondary")
                            export_button = ui.button(
                                "开始导出",
                                on_click=lambda: run_in_thread("export"),
                            ).props("unelevated color=primary")

                    with ui.card().classes("glass-card w-full p-6 gap-4"):
                        ui.label("运行日志").classes("text-lg font-semibold text-slate-900")
                        log_view = ui.textarea(
                            value="",
                            placeholder="日志会显示在这里",
                        ).props("readonly autogrow outlined").classes("w-full min-h-[360px]")

    sql_panel.style("display: block;")
    maxcompute_panel.style("display: none;")
    partition_row.style("display: none;")
    ui.timer(0.3, process_events)
    ui.run(
        native=True,
        reload=False,
        title="Parquet Export Studio",
        fullscreen=False,
        window_size=(1320, 860),
    )


def _backend_hint(backend: Backend) -> str:
    if backend is Backend.ORACLE:
        return "Oracle 默认使用 python-oracledb Thin 模式，填写 Service Name 即可。"
    if backend is Backend.MYSQL:
        return "MySQL 采用服务端游标流式读取，适合大表导出。"
    if backend is Backend.POSTGRESQL:
        return "PostgreSQL 使用 named cursor 分批拉取结果，避免客户端一次性缓存。"
    return "MaxCompute 通过表读取接口导出 Arrow RecordBatch，再写入本地 Parquet。"
