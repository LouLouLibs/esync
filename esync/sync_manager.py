import threading
import queue
import subprocess
from pathlib import Path
from typing import Optional
from rich.console import Console
from .config import SyncConfig

console = Console()

class SyncManager:
    def __init__(self, config: SyncConfig):
        self._sync_lock = threading.Lock()
        self._task_queue = queue.Queue()
        self._current_sync: Optional[subprocess.Popen] = None
        self._should_stop = threading.Event()
        self._sync_thread = threading.Thread(target=self._sync_worker, daemon=True)
        self._config = config
        self._sync_thread.start()

    @property
    def config(self) -> SyncConfig:
        return self._config

    def schedule_sync(self, path: Path):
        """Schedule a sync task."""
        self._task_queue.put(path)

    def stop(self):
        """Stop the sync manager and cleanup."""
        self._should_stop.set()
        if self._current_sync:
            self._current_sync.terminate()
        self._sync_thread.join()

    def _sync_worker(self):
        while not self._should_stop.is_set():
            try:
                path = self._task_queue.get(timeout=1.0)
            except queue.Empty:
                continue

            with self._sync_lock:
                if self._current_sync is not None:
                    console.print("[yellow]Sync already in progress, queuing changes...[/]")
                    try:
                        self._current_sync.wait()
                    except subprocess.CalledProcessError as e:
                        console.print(f"[bold red]Previous sync failed: {e}[/]")

                try:
                    self._perform_sync(path)
                except Exception as e:
                    console.print(f"[bold red]Sync error: {e}[/]")

            self._task_queue.task_done()

    def _perform_sync(self, path: Path):
        """Perform the actual sync operation."""
        cmd = self._build_rsync_command(path)

        with console.status("[bold green]Syncing...") as status:
            self._current_sync = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )

            try:
                stdout, stderr = self._current_sync.communicate()
                if self._current_sync.returncode != 0:
                    raise subprocess.CalledProcessError(
                        self._current_sync.returncode, cmd, stdout, stderr
                    )
                console.print("[bold green]✓[/] Sync completed successfully!")
            finally:
                self._current_sync = None

    def _build_rsync_command(self, source_path: Path) -> list[str]:
        """Build rsync command for local or remote sync."""
        cmd = [
            "rsync",
            "-rtv",  # recursive, preserve times, verbose
            "--progress",  # show progress
            # "--backup",    # make backups of deleted files
            # "--backup-dir=.rsync_backup",  # backup directory
        ]

        # Add ignore patterns
        for pattern in self._config.ignores:
            # Remove any quotes and brackets from the input
            clean_pattern = pattern.strip('"[]\'')
            # Handle **/ pattern (recursive)
            if clean_pattern.startswith('**/'):
                clean_pattern = clean_pattern[3:]  # Remove **/ prefix
            cmd.extend(["--exclude", clean_pattern])

        # Ensure we have absolute paths
        source = f"{source_path.absolute()}/"

        if self._config.is_remote():
            # For remote sync
            ssh = self._config.ssh
            if ssh.username:
                remote = f"{ssh.username}@{ssh.host}:{self._config.target}"
            else:
                remote = f"{ssh.host}:{self._config.target}"
            cmd.append(source)
            cmd.append(remote)
        else:
            # For local sync
            target = self._config.target
            if isinstance(target, Path):
                target = target.absolute()
                target.mkdir(parents=True, exist_ok=True)
            cmd.append(source)
            cmd.append(str(target) + '/')

        console.print(f"Running rsync command: {' '.join(cmd)}")
        return cmd
