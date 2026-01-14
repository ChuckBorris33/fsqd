import asyncio
import logging
import os
import threading
import time
from typing import Dict, Optional
from urllib.parse import urljoin

import aiohttp
from bs4 import BeautifulSoup

from fsqd.config import (
    DOWNLOAD_CHUNK_SIZE,
    DOWNLOAD_DIR,
    PROGRESS_UPDATE_INTERVAL,
    USER_AGENT,
)
from fsqd.storage import (
    QueueItem,
    get_active,
    mark_completed,
    mark_downloading,
    mark_failed,
    update_item_progress,
)

# In-memory cancellation events: {item_id: asyncio.Event}
download_cancel_events: Dict[str, asyncio.Event] = {}
download_cancel_events_lock = asyncio.Lock()


async def download_file(item_id: str, url: str) -> None:
    """Download file from FastShare link"""
    logging.info(f"Starting download for item {item_id}")
    item = await _get_pending_item(item_id)
    if not item:
        logging.warning(f"Item {item_id} not found or not pending")
        return

    await mark_downloading(item_id)

    try:
        cancel_event = await _setup_cancel_event(item_id)
        await update_item_progress(item_id, 0, "")

        async with aiohttp.ClientSession() as session:
            form_action = await _get_download_form(session, url)
            if not form_action:
                logging.error(f"Could not get download form for {url}")
                await mark_failed(item_id, "Could not get download form")
                return

            await _perform_download(session, item_id, form_action, item, cancel_event)
    except Exception as e:
        logging.error(f"Error downloading {item_id}: {e}")
        await mark_failed(item_id, str(e))
    finally:
        await _cleanup_cancel_event(item_id)


async def _get_pending_item(item_id: str) -> Optional[QueueItem]:
    active = await get_active()
    item = next((i for i in active if i["id"] == item_id), None)
    if not item or item.get("status", "pending") != "pending":
        return None
    return item


async def _setup_cancel_event(item_id: str) -> asyncio.Event:
    async with download_cancel_events_lock:
        cancel_event = asyncio.Event()
        download_cancel_events[item_id] = cancel_event
    return cancel_event


async def _cleanup_cancel_event(item_id: str) -> None:
    async with download_cancel_events_lock:
        if item_id in download_cancel_events:
            del download_cancel_events[item_id]


async def _get_download_form(session: aiohttp.ClientSession, url: str) -> Optional[str]:
    headers = {"User-Agent": USER_AGENT}
    async with session.get(url, headers=headers):
        pass

    async with session.get(url, headers=headers) as response:
        html = await response.text()
        soup = BeautifulSoup(html, "html.parser")

    form = soup.find("form")
    if not form:
        return None

    form_action = form.get("action", "")
    if not form_action or not isinstance(form_action, str):
        return None

    if form_action.startswith("/free/"):
        return urljoin(url, form_action)

    for f in soup.find_all("form"):
        action = f.get("action", "")
        if isinstance(action, str) and action.startswith("/free/"):
            return urljoin(url, action)

    return None


def _format_speed(speed_bps: float) -> str:
    if speed_bps >= 1024 * 1024:
        return f"{speed_bps / (1024 * 1024):.1f} MB/s"
    elif speed_bps >= 1024:
        return f"{speed_bps / 1024:.1f} KB/s"
    else:
        return f"{speed_bps:.1f} B/s"


async def _download_file_content(
    response: aiohttp.ClientResponse,
    item_id: str,
    item: QueueItem,
    cancel_event: asyncio.Event,
) -> None:
    content_type = response.headers.get("Content-Type", "").lower()
    
    # Check for valid content types
    if not (
        "application/octet-stream" in content_type
        or "application/force-download" in content_type
        or content_type.startswith("video/")
        or content_type.startswith("audio/")
    ):
        logging.warning(f"Unexpected content type: {content_type}")
        if "text/html" in content_type:
            # HTML response indicates we need to retry
            await _handle_html_response(response, item_id, item, cancel_event)
            return
        else:
            await mark_failed(item_id, f"Invalid content type: {content_type}")
            return

    # Rest of the existing download logic...
    filename = item.get("title", "download")
    filename = "".join(
        c for c in filename if c.isalnum() or c in (" ", "-", "_", ".")
    ).rstrip()
    os.makedirs(DOWNLOAD_DIR, exist_ok=True)
    filepath = os.path.join(DOWNLOAD_DIR, filename)
    logging.info(f"Saving download to {filepath}")

    total_size = int(response.headers.get("Content-Length", 0))
    downloaded = 0
    last_update_time = time.time()
    bytes_since_last_update = 0
    last_progress = -1

    with open(filepath, "wb") as f:
        async for chunk in response.content.iter_chunked(DOWNLOAD_CHUNK_SIZE):
            if cancel_event.is_set():
                logging.info(f"Download cancelled by user: {item_id}")
                await mark_failed(item_id, "Download cancelled by user")
                return
            f.write(chunk)
            downloaded += len(chunk)
            bytes_since_last_update += len(chunk)

            current_time = time.time()
            time_since_update = current_time - last_update_time

            # Update progress at least every second
            if time_since_update >= PROGRESS_UPDATE_INTERVAL:
                speed = "0 B/s"
                if time_since_update > 0:
                    speed_bps = bytes_since_last_update / time_since_update
                    speed = _format_speed(speed_bps)

                progress = None
                if total_size > 0:
                    progress = int((downloaded / total_size) * 100)

                await update_item_progress(item_id, progress, speed)

                last_update_time = current_time
                bytes_since_last_update = 0
                if progress:
                    last_progress = progress

            # Also update progress if it changes significantly, even if less than a second
            elif total_size > 0:
                progress = int((downloaded / total_size) * 100)
                if progress > last_progress:
                    await update_item_progress(item_id, progress)
                    last_progress = progress

    await update_item_progress(item_id, 100, "")
    await mark_completed(item_id)
    logging.info(f"Download completed for item {item_id}")


async def _handle_html_response(
    response: aiohttp.ClientResponse,
    item_id: str,
    item: QueueItem,
    cancel_event: asyncio.Event,
    retry_count: int = 0,
    max_retries: int = 3,
) -> None:
    if retry_count >= max_retries:
        await mark_failed(item_id, "Max retries reached for HTML response")
        return

    # Exponential backoff: 2^retry_count seconds
    wait_time = 2 ** retry_count
    logging.info(f"Received HTML response, retrying in {wait_time} seconds...")
    await asyncio.sleep(wait_time)

    # Retry the download
    async with aiohttp.ClientSession() as session:
        form_action = await _get_download_form(session, item["url"])
        if not form_action:
            await mark_failed(item_id, "Could not get download form after retry")
            return

        try:
            headers = {"User-Agent": USER_AGENT}
            async with session.post(
                form_action, data={}, headers=headers, timeout=None
            ) as new_response:
                if new_response.status != 200:
                    await mark_failed(item_id, f"Retry failed: {new_response.status}")
                    return

                await _download_file_content(new_response, item_id, item, cancel_event)
        except Exception as e:
            logging.error(f"Retry failed: {e}")
            await _handle_html_response(response, item_id, item, cancel_event, retry_count + 1)


async def _perform_download(
    session: aiohttp.ClientSession,
    item_id: str,
    form_action: str,
    item: QueueItem,
    cancel_event: asyncio.Event,
) -> None:
    headers = {"User-Agent": USER_AGENT}
    async with session.post(
        form_action, data={}, headers=headers, timeout=None
    ) as response:
        if response.status != 200:
            logging.error(f"Download request failed for {item_id}: {response.status}")
            await mark_failed(item_id, f"Download request failed: {response.status}")
            return

        await _download_file_content(response, item_id, item, cancel_event)


async def download_worker() -> None:
    """Background worker that processes the download queue"""
    logging.info("Download worker started")
    while True:
        try:
            active = await get_active()

            # Find first pending item
            pending_item: Optional[QueueItem] = None
            for item in active:
                status = item.get("status", "pending")
                if status == "pending":
                    pending_item = item
                    break

            if pending_item:
                logging.info(f"Processing item {pending_item['id']}")
                # Download the file
                await download_file(pending_item["id"], pending_item["url"])
                await asyncio.sleep(30)  # Short pause before next item
            else:
                # No pending items, wait
                await asyncio.sleep(2)

        except Exception as e:
            logging.error(f"Error in download worker: {e}")
            await asyncio.sleep(5)


def start_download_worker() -> None:
    """Start the download worker in a separate thread"""
    logging.info("Starting download worker thread")

    def run_worker():
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        loop.run_until_complete(download_worker())

    worker_thread = threading.Thread(target=run_worker, daemon=True)
    worker_thread.start()


async def cancel_download(item_id: str) -> None:
    """Cancel a currently downloading item"""
    logging.info(f"Cancelling download for item {item_id}")
    # Signal cancellation
    async with download_cancel_events_lock:
        event = download_cancel_events.get(item_id)
        if event:
            event.set()
    # Mark as failed (if running, download_file will see the event and mark as failed)
    await mark_failed(item_id, "Download cancelled by user")
    await update_item_progress(item_id, 0, "")
