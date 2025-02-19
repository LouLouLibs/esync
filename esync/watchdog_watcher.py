from pathlib import Path
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler
from .watcher_base import WatcherBase
from .sync_manager import SyncManager
import fnmatch

class WatchdogHandler(FileSystemEventHandler):
    def __init__(self, root_path: Path, sync_manager: SyncManager):
        self.root_path = root_path
        self.sync_manager = sync_manager
        self.ignores = sync_manager.config.ignores

    def should_ignore(self, path: str) -> bool:
        # Convert path to relative path from root for matching
        try:
            rel_path = Path(path).relative_to(self.root_path)
            return any(fnmatch.fnmatch(str(rel_path), pattern.strip('"[]\''))
                      for pattern in self.ignores)
        except ValueError:
            return False

    def on_any_event(self, event):
        # Skip directories and temporary files
        if event.is_directory or any(
            event.src_path.endswith(p) for p in ['.swp', '.swx', '~']
        ):
            return

        # Check if file should be ignored
        if self.should_ignore(event.src_path):
            print(f"Ignoring change to {event.src_path}")
            return

        # Only sync if file isn't ignored
        self.sync_manager.schedule_sync(self.root_path)

class WatchdogWatcher(WatcherBase):
    def __init__(self, root: Path, sync_manager: SyncManager):
        super().__init__(root, sync_manager)
        self.handler = WatchdogHandler(root, sync_manager)
        self.observer = Observer()

    def start(self) -> None:
        """Start watching for changes."""
        self.observer.schedule(self.handler, str(self.root), recursive=True)
        self.observer.start()

    def stop(self) -> None:
        """Stop watching for changes."""
        self.observer.stop()
        self.observer.join()
