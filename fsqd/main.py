import uuid
from datetime import datetime

import jinja2
from aiohttp import web
from aiohttp_jinja2 import setup as jinja_setup
from aiohttp_jinja2 import template as jinja_template

from fsqd.download import (
    cancel_download,
    start_download_worker,
)
from fsqd.metadata import extract_file_info
from fsqd.storage import (
    QueueItem,
    add_to_queue,
    clear_completed,
    clear_failed,
    get_active,
    get_all,
    move_in_queue,
    progress_store,
    progress_store_lock,
    remove_from_queue,
    reset_stuck_downloads,
    retry_failed,
)

# --- Route Handlers ---


async def index(request: web.Request) -> web.FileResponse:
    return web.FileResponse("./static/index.html")


async def health_check(request: web.Request) -> web.Response:
    return web.Response(text="OK")


@jinja_template("queue.html")
async def get_queue(request: web.Request) -> dict:
    # Fetch all lists for the view
    lists = await get_all()
    merged = {}
    with progress_store_lock:
        for key, items in lists.items():
            merged[key] = []
            for item in items:
                merged_item = dict(item)
                progress_info = progress_store.get(item["id"])
                merged_item["progress"] = (
                    progress_info.get("progress", 0) if progress_info else 0
                )
                merged_item["speed"] = (
                    progress_info.get("speed", "") if progress_info else ""
                )
                merged[key].append(merged_item)
    # For the view: show downloaded, then active, then failed, then completed
    return {
        "downloaded": merged["downloaded"],
        "active": merged["active"],
        "failed": merged["failed"],
        "completed": merged["completed"],
    }


async def add_to_queue_handler(request: web.Request) -> web.StreamResponse:
    data = await request.post()
    url_val = data.get("link-input", "")
    url = url_val.decode() if isinstance(url_val, bytes) else str(url_val)
    url = url.strip()

    if not url:
        return web.Response(text="URL is required", status=400)
    if "fastshare.cloud" not in url:
        return web.Response(text="Only fastshare.cloud links are allowed", status=400)

    title, size = await extract_file_info(url)

    new_item: QueueItem = {
        "id": str(uuid.uuid4()),
        "url": url,
        "title": title,
        "size": size,
        "status": "pending",
        "added_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "error": None,
    }
    await add_to_queue(new_item)
    with progress_store_lock:
        progress_store[new_item["id"]] = {"progress": 0, "speed": ""}
    return await get_queue(request)


async def remove_item_handler(request: web.Request) -> web.StreamResponse:
    data = await request.post()
    item_id = str(data.get("item_id", "")).strip()
    if not item_id:
        return web.Response(text="Item ID is required", status=400)
    await remove_from_queue(item_id)
    return await get_queue(request)


async def move_item_handler(request: web.Request) -> web.StreamResponse:
    data = await request.post()
    item_id = str(data.get("item_id", "")).strip()
    direction = str(data.get("direction", "")).strip()
    if not item_id or direction not in ["up", "down"]:
        return web.Response(text="Invalid parameters", status=400)
    await move_in_queue(item_id, direction)
    return await get_queue(request)


async def clear_failed_handler(request: web.Request) -> web.StreamResponse:
    await clear_failed()
    return await get_queue(request)


async def clear_completed_handler(request: web.Request) -> web.StreamResponse:
    await clear_completed()
    return await get_queue(request)


async def retry_item_handler(request: web.Request) -> web.StreamResponse:
    data = await request.post()
    item_id = str(data.get("item_id", "")).strip()
    if not item_id:
        return web.Response(text="Item ID is required", status=400)
    await retry_failed(item_id)
    with progress_store_lock:
        progress_store[item_id] = {"progress": 0, "speed": ""}
    return await get_queue(request)


async def cancel_download_handler(request: web.Request) -> web.StreamResponse:
    data = await request.post()
    item_id = str(data.get("item_id", "")).strip()
    if not item_id:
        return web.Response(text="Item ID is required", status=400)
    await cancel_download(item_id)
    return await get_queue(request)


# --- App Factory and Entrypoint ---


def create_app() -> web.Application:
    app = web.Application()
    jinja_setup(app, loader=jinja2.FileSystemLoader("templates"))
    app.router.add_get("/", index)
    app.router.add_get("/healthz", health_check)
    app.router.add_get("/api/queue", get_queue)
    app.router.add_post("/api/queue", add_to_queue_handler)
    app.router.add_post("/api/queue/remove", remove_item_handler)
    app.router.add_post("/api/queue/move", move_item_handler)
    app.router.add_post("/api/queue/clear_failed", clear_failed_handler)
    app.router.add_post("/api/queue/clear_completed", clear_completed_handler)
    app.router.add_post("/api/queue/retry", retry_item_handler)
    app.router.add_post("/api/queue/cancel", cancel_download_handler)
    app.router.add_static("/static", path="./static", name="static")

    async def cleanup_on_shutdown(app):
        try:
            # Reset progress for all active items
            active = await get_active()
            with progress_store_lock:
                for item in active:
                    progress_store[item["id"]] = {"progress": 0}

        except Exception:
            pass

        app.on_cleanup.append(cleanup_on_shutdown)

    return app


if __name__ == "__main__":
    import asyncio
    from pathlib import Path

    from fsqd.config import DOWNLOAD_DIR, HOST, PORT

    # --- Initialization ---
    downloads_path = Path(DOWNLOAD_DIR)
    downloads_path.mkdir(exist_ok=True)

    asyncio.run(reset_stuck_downloads())
    start_download_worker()

    app = create_app()
    app.router.add_static("/downloads", path=downloads_path, name="downloads")

    print(f"FSQD Server running on http://{HOST}:{PORT}")
    print(f"Downloads directory: {downloads_path.resolve()}")

    web.run_app(app, host=HOST, port=PORT)
