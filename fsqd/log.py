import logging
import sys

from fsqd.config import LOG_LEVEL


def setup_logging():
    """
    Configures the logging for the application.
    """
    logging.basicConfig(
        level=LOG_LEVEL,
        format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
        stream=sys.stdout,
    )
