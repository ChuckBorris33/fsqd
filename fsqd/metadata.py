import logging

import aiohttp
from bs4 import BeautifulSoup


async def extract_file_info(url: str) -> tuple[str, str]:
    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
    }
    logging.info(f"Extracting file info from {url}")
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(url, headers=headers) as response:
                if response.status != 200:
                    logging.error(f"Failed to fetch {url}, status: {response.status}")
                    return "Unknown", "Unknown size"
                html = await response.text()
                soup = BeautifulSoup(html, "html.parser")
                title_elem = soup.find("h1", class_="section_title")
                if not title_elem:
                    logging.warning(f"Title not found for {url}")
                title = title_elem.get_text(strip=True) if title_elem else "Unknown"
                size_elem = soup.find("td", class_="footer-video-size")
                if not size_elem:
                    logging.warning(f"Size not found for {url}")
                size = size_elem.get_text(strip=True) if size_elem else "Unknown size"
                logging.info(f"Found title: {title}, size: {size}")
                return title, size
    except Exception as e:
        logging.error(f"Error extracting file info from {url}: {e}")
        return "Unknown", "Unknown size"
