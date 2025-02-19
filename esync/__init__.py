"""
esync - File synchronization tool with watchdog/watchman support
"""

from .config import SyncConfig, SSHConfig
from .sync_manager import SyncManager
from .watcher_base import WatcherBase
from .watchdog_watcher import WatchdogWatcher
from .watchman_watcher import WatchmanWatcher

__version__ = "0.1.0"

__all__ = [
    "SyncConfig",
    "SSHConfig",
    "SyncManager",
    "WatcherBase",
    "WatchdogWatcher",
    "WatchmanWatcher",
]
