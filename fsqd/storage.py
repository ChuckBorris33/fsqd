import threading
from typing import Dict, List, Optional, TypedDict, cast

import diskcache

from fsqd.config import ACTIVE_KEY, CACHE_DIR, COMPLETED_KEY, DOWNLOADED_KEY, FAILED_KEY

# --- Typing ---


class QueueItem(TypedDict):
    id: str
    url: str
    title: str
    size: str
    status: str
    added_at: str
    error: Optional[str]


class ProgressInfo(TypedDict, total=False):
    progress: int
    speed: str


# ... (typing classes) ...

# Initialize cache from config
queue_cache = diskcache.Cache(CACHE_DIR)

# In-memory progress store: {item_id: ProgressInfo}
progress_store: Dict[str, ProgressInfo] = {}
progress_store_lock = threading.Lock()


async def _load_list(key: str) -> List[QueueItem]:
    # In the future, replace with async cache/file/db access if needed
    items = queue_cache.get(key, [])
    if not isinstance(items, list):
        return []
    clean_list: List[QueueItem] = []
    for item in items:
        if isinstance(item, dict):
            item = dict(item)
            item.pop("progress", None)
            clean_list.append(cast(QueueItem, item))
    return clean_list


async def _save_list(key: str, items: List[QueueItem]) -> None:
    # In the future, replace with async cache/file/db access if needed
    to_save: List[QueueItem] = []
    for item in items:
        item_copy = dict(item)
        item_copy.pop("progress", None)
        to_save.append(cast(QueueItem, item_copy))
    queue_cache.set(key, to_save)


async def get_active() -> List[QueueItem]:
    return await _load_list(ACTIVE_KEY)


async def get_failed() -> List[QueueItem]:
    return await _load_list(FAILED_KEY)


async def get_completed() -> List[QueueItem]:
    return await _load_list(COMPLETED_KEY)


async def get_downloaded() -> List[QueueItem]:
    return await _load_list(DOWNLOADED_KEY)


async def get_all() -> Dict[str, List[QueueItem]]:
    return {
        "active": await get_active(),
        "failed": await get_failed(),
        "completed": await get_completed(),
        "downloaded": await get_downloaded(),
    }


def _find_index(items: List[QueueItem], item_id: str) -> Optional[int]:
    for i, item in enumerate(items):
        if item["id"] == item_id:
            return i
    return None


async def add_to_queue(item: QueueItem) -> None:
    active = await get_active()
    active.append(item)
    await _save_list(ACTIVE_KEY, active)


async def remove_from_queue(item_id: str) -> None:
    active = await get_active()
    idx = _find_index(active, item_id)
    if idx is not None:
        del active[idx]
        await _save_list(ACTIVE_KEY, active)


async def move_in_queue(item_id: str, direction: str) -> None:
    active = await get_active()
    idx = _find_index(active, item_id)
    if idx is None:
        return
    if direction == "up" and idx > 0:
        active[idx], active[idx - 1] = active[idx - 1], active[idx]
    elif direction == "down" and idx < len(active) - 1:
        active[idx], active[idx + 1] = active[idx + 1], active[idx]
    await _save_list(ACTIVE_KEY, active)


async def mark_downloading(item_id: str) -> None:
    active = await get_active()
    idx = _find_index(active, item_id)
    if idx is not None:
        item = active[idx]
        item["status"] = "downloading"
        await _save_list(ACTIVE_KEY, active)


async def mark_completed(item_id: str) -> None:
    active = await get_active()
    idx = _find_index(active, item_id)
    if idx is not None:
        item = active.pop(idx)
        item["status"] = "completed"
        item["error"] = None
        completed = await get_completed()
        completed.append(item)
        await _save_list(ACTIVE_KEY, active)
        await _save_list(COMPLETED_KEY, completed)


async def mark_failed(item_id: str, error: str) -> None:
    active = await get_active()
    idx = _find_index(active, item_id)
    if idx is not None:
        item = active.pop(idx)
        item["status"] = "failed"
        item["error"] = error
        failed = await get_failed()
        failed.append(item)
        await _save_list(ACTIVE_KEY, active)
        await _save_list(FAILED_KEY, failed)


async def mark_downloaded(item_id: str) -> None:
    completed = await get_completed()
    idx = _find_index(completed, item_id)
    if idx is not None:
        item = completed.pop(idx)
        item["status"] = "downloaded"
        item["error"] = None
        downloaded = await get_downloaded()
        downloaded.append(item)
        await _save_list(COMPLETED_KEY, completed)
        await _save_list(DOWNLOADED_KEY, downloaded)


async def retry_failed(item_id: str) -> None:
    failed = await get_failed()
    idx = _find_index(failed, item_id)
    if idx is not None:
        item = failed.pop(idx)
        item["status"] = "pending"
        item["error"] = None
        active = await get_active()
        active.append(item)
        await _save_list(FAILED_KEY, failed)
        await _save_list(ACTIVE_KEY, active)


async def clear_failed() -> None:
    await _save_list(FAILED_KEY, [])


async def clear_completed() -> None:
    await _save_list(COMPLETED_KEY, [])


async def reset_stuck_downloads() -> None:
    active = await get_active()
    changed = False
    for item in active:
        if item.get("status") == "downloading":
            item["status"] = "pending"
            item["error"] = "Download interrupted - please retry"
            changed = True
    if changed:
        await _save_list(ACTIVE_KEY, active)

    # Reset progress in memory
    with progress_store_lock:
        for item in active:
            if item.get("status") == "pending":
                progress_store[item["id"]] = {"progress": 0, "speed": ""}


async def find_item_by_id(item_id: str) -> Optional[QueueItem]:
    # Search all lists
    for getter in [get_active, get_failed, get_completed, get_downloaded]:
        items = await getter()
        for item in items:
            if item["id"] == item_id:
                return item
    return None


async def update_item_progress(
    item_id: str,
    progress: Optional[int] = None,
    speed: Optional[str] = None,
) -> None:
    # Update progress and/or speed in memory
    with progress_store_lock:
        if item_id not in progress_store:
            progress_store[item_id] = {"progress": 0, "speed": ""}
        if progress is not None:
            progress_store[item_id]["progress"] = progress
        if speed is not None:
            progress_store[item_id]["speed"] = speed
