# esync/watchman_watcher.py
import pywatchman
from pathlib import Path
from rich.console import Console
from .watcher_base import WatcherBase
from .sync_manager import SyncManager
import time

console = Console()

class WatchmanWatcher(WatcherBase):
    def __init__(self, root: Path, sync_manager: SyncManager):
        super().__init__(root, sync_manager)
        self.client = pywatchman.client(timeout=5.0)
        self.watch = None
        self._stop = False

    def start(self) -> None:
        """Start watching for changes."""
        try:
            self.client.capabilityCheck(required=['relative_root'])

            root_path = str(self.root.absolute().resolve())
            console.print(f"Starting watchman on {root_path}")

            watch_response = self.client.query('watch-project', root_path)
            self.watch = watch_response['watch']
            relative_path = watch_response.get('relative_path')

            clock = self.client.query('clock', self.watch)['clock']

            expr = self.build_ignore_expression(self.sync_manager.config.ignores)

            sub = {
                'expression': expr,
                'fields': ['name', 'exists', 'type'],
                'since': clock
            }

            if relative_path:
                sub['relative_root'] = relative_path

            self.client.query('subscribe', self.watch, 'sync-subscription', sub)
            console.print("Watchman subscription established")

            while not self._stop:
                try:
                    data = self.client.receive()
                    if data and 'subscription' in data:
                        files = data.get('files', [])
                        if files:
                            console.print(f"Changes detected: {files}")
                            self.sync_manager.schedule_sync(self.root)
                except pywatchman.SocketTimeout:
                    continue
                except Exception as e:
                    console.print(f"[red]Error processing watchman event: {e}[/red]")
                    if not isinstance(e, pywatchman.SocketTimeout):
                        break

                time.sleep(0.1)

        except Exception as e:
            console.print(f"[red]Failed to initialize Watchman: {e}[/red]")
            try:
                version = self.client.query('version')
                console.print(f"Watchman version: {version}")
            except Exception as ve:
                console.print(f"Could not get version: {ve}")
            raise

    def stop(self) -> None:
        """Stop watching for changes."""
        self._stop = True
        if self.watch:
            try:
                self.client.query('unsubscribe', self.watch, 'sync-subscription')
            except:
                pass
        try:
            self.client.close()
        except:
            pass

    def build_ignore_expression(self, ignores: list[str]) -> list:
        """Build watchman ignore expression from ignore patterns."""
        not_expressions = [
            ['not', ['match', pattern.strip('"[]\'')]]
            for pattern in ignores
        ]
        return ['allof',
            ['type', 'f'],  # only watch files
            *not_expressions,  # add all ignore patterns
            ['not', ['match', '*.swp']],
            ['not', ['match', '*.swx']],
            ['not', ['match', '.git/*']],
            ['not', ['match', '__pycache__/*']],
            ['not', ['match', '*.pyc']]
        ]
