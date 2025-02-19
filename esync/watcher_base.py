from abc import ABC, abstractmethod
from pathlib import Path
from .sync_manager import SyncManager

class WatcherBase(ABC):
    def __init__(self, root: Path, sync_manager: SyncManager):
        self.root = root
        self.sync_manager = sync_manager

    @abstractmethod
    def start(self) -> None:
        """Start watching for changes."""
        pass

    @abstractmethod
    def stop(self) -> None:
        """Stop watching for changes."""
        pass
