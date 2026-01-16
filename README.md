# fsqd - FastShare Queue Downloader

**fsqd** is a simple, web-based queue manager for downloading files from `fastshare.cloud`.

<img width="1001" height="959" alt="Screenshot From 2026-01-15 12-29-19" src="https://github.com/user-attachments/assets/c346f519-f94d-4532-a1b2-e5b52e1b35a6" />

## Disclaimer

This project was created for personal use. There is no intention to maintain it outside of my personal usage needs.

## Features

*   **Web-based UI:** Easy-to-use interface for managing downloads.
*   **Download Queue:** Add multiple `fastshare.cloud` links to a queue.
*   **Queue Management:**
    *   View active, completed, failed, and pending downloads.
    *   Real-time progress and download speed.
    *   Cancel active downloads.
    *   Retry failed downloads.
    *   Clear completed and failed lists.
    *   Re-order items in the pending queue.
*   **Persistent Storage:** Download queue and files are saved between sessions.

## How to Run

The easiest way to run `fsqd` is with Docker.

1.  **Clone the repository:**
    ```bash
    git clone <repository-url>
    cd fsqd
    ```

2.  **Run with Docker Compose:**
    ```bash
    docker-compose up -d
    ```

3.  **Access the application:**
    Open your web browser and navigate to `http://localhost:8080`.

The downloaded files will be available in the `./data/downloads` directory on your host machine.

## Usage

### Adding a Download

1.  Paste a `fastshare.cloud` URL into the input field at the top of the page.
2.  Click the "Add to Queue" button.
3.  The file will be added to the "Pending" queue and the download will start automatically when a worker is free.

### Managing the Queue

*   **Cancel:** Click the "Cancel" button next to an active download to stop it. The item will be moved to the "Failed" list.
*   **Retry:** Click the "Retry" button next to a failed download to add it back to the pending queue.
*   **Remove:** Click the "Remove" button to permanently delete an item from any list.
*   **Clear Lists:** Use the "Clear Completed" or "Clear Failed" buttons to empty those entire sections.
*   **Re-order:** Use the up and down arrows next to items in the "Pending" queue to change their download priority.
