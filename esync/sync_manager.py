import threading
import queue
import subprocess
import datetime
import os
import logging
import re
from pathlib import Path
from typing import Optional, List, Union
from rich.console import Console
from rich.panel import Panel
from rich.text import Text
from .config import SyncConfig

console = Console()
# console = Console(stderr=True, log_time=True, log_path=False) # for debugging


# Customize logger to use shorter log level names
class CustomAdapter(logging.LoggerAdapter):
    def process(self, msg, kwargs):
        return msg, kwargs

class ShortLevelNameFormatter(logging.Formatter):
    """Custom formatter with shorter level names"""
    short_levels = {
        'DEBUG': 'DEBUG',
        'INFO': 'INFO',
        'WARNING': 'WARN',
        'ERROR': 'ERROR',
        'CRITICAL': 'CRITIC'
    }

    def format(self, record):
        if record.levelname in self.short_levels:
            record.levelname = self.short_levels[record.levelname]
        return super().format(record)

class SyncManager:
    """Manages file synchronization operations."""

    def __init__(self, config: SyncConfig, log_file: Optional[str] = None):
        """Initialize the sync manager.

        Args:
            config: The sync configuration
            log_file: Optional path to log file
        """
        self._sync_lock = threading.Lock()
        self._task_queue = queue.Queue()
        self._current_sync: Optional[subprocess.Popen] = None
        self._should_stop = threading.Event()
        self._sync_thread = threading.Thread(target=self._sync_worker, daemon=True)
        self._config = config
        self._last_sync_time = None
        self._last_sync_status = None
        self._sync_count = 0

        # Default to quiet mode (cleaner output)
        self._quiet = True
        self._verbose = False

        # Set up logging if log file is specified
        self._logger = None
        if log_file:
            self._setup_logging(log_file)

        # Set verbose/quiet mode based on config
        if hasattr(config, 'verbose'):
            self._verbose = config.verbose
            self._quiet = not config.verbose

        self._sync_thread.start()

        # Single status panel that we'll update
        self._status_panel = Panel(
            Text("Waiting for changes...", style="italic dim"),
            title="Sync Status"
        )

    def _setup_logging(self, log_file: str):
        """Set up logging to file."""
        self._logger = logging.getLogger("esync")
        self._logger.setLevel(logging.DEBUG)  # Always use DEBUG level for logging

        # Create handlers
        file_handler = logging.FileHandler(log_file)
        file_handler.setLevel(logging.DEBUG)  # Always log at DEBUG level

        # Create formatters with fixed-width level names using our custom formatter
        formatter = ShortLevelNameFormatter(
            '%(asctime)s - %(name)s - %(levelname)-5s - %(message)s',
            datefmt='%Y-%m-%d %H:%M:%S'
        )
        file_handler.setFormatter(formatter)

        # Add handlers to the logger
        self._logger.addHandler(file_handler)
        self._logger.info("ESync started")
        self._logger.info(f"Log level set to: DEBUG")

    def _log(self, level: str, message: str):
        """Log a message if logging is enabled."""
        if self._logger:
            if level.lower() == "info":
                self._logger.info(message)
            elif level.lower() == "warning" or level.lower() == "warn":
                self._logger.warning(message)
            elif level.lower() == "error" or level.lower() == "err":
                self._logger.error(message)
            elif level.lower() == "debug" or level.lower() == "dbg":
                self._logger.debug(message)

    @property
    def config(self) -> SyncConfig:
        return self._config

    @property
    def last_sync_time(self) -> Optional[datetime.datetime]:
        return self._last_sync_time

    @property
    def last_sync_status(self) -> Optional[bool]:
        return self._last_sync_status

    @property
    def sync_count(self) -> int:
        return self._sync_count

    @property
    def status_panel(self) -> Panel:
        return self._status_panel

    def schedule_sync(self, path: Path):
        """Schedule a sync task."""
        self._task_queue.put(path)
        self._log("info", f"Sync scheduled for {path}")

    def stop(self):
        """Stop the sync manager and cleanup."""
        self._should_stop.set()
        self._log("info", "Stopping sync manager")
        if self._current_sync:
            self._current_sync.terminate()
        self._sync_thread.join()

    def _update_status(self, status_text: Text):
        """Update the status panel with new text."""
        self._status_panel = Panel(status_text, title="Sync Status")

    def _sync_worker(self):
        while not self._should_stop.is_set():
            try:
                path = self._task_queue.get(timeout=1.0)
            except queue.Empty:
                continue

            with self._sync_lock:
                if self._current_sync is not None:
                    status_text = Text()
                    status_text.append("ESync: ", style="bold cyan")
                    status_text.append(f"Sync #{self._sync_count} ", style="yellow")
                    status_text.append("in progress - ", style="italic")
                    status_text.append("waiting for previous sync to complete", style="italic yellow")
                    self._update_status(status_text)

                    self._log("warn", "Sync already in progress, queuing changes...")
                    if not self._quiet:
                        console.print("[yellow]Sync already in progress, queuing changes...[/]", highlight=False)

                    try:
                        self._current_sync.wait()
                    except subprocess.CalledProcessError as e:
                        error_msg = f"Previous sync failed: {e}"
                        self._log("error", error_msg)
                        if not self._quiet:
                            console.print(f"[bold red]{error_msg}[/]", highlight=False)

                try:
                    self._perform_sync(path)
                except Exception as e:
                    error_msg = f"Sync error: {e}"
                    self._log("error", error_msg)
                    if not self._quiet:
                        console.print(f"[bold red]{error_msg}[/]", highlight=False)
                    self._last_sync_status = False

            self._task_queue.task_done()

    def _format_size(self, size_bytes: int) -> str:
        """Format bytes into human readable size."""
        if size_bytes < 1024:
            return f"{size_bytes} B"
        elif size_bytes < 1024 * 1024:
            return f"{size_bytes/1024:.1f} KB"
        elif size_bytes < 1024 * 1024 * 1024:
            return f"{size_bytes/(1024*1024):.1f} MB"
        else:
            return f"{size_bytes/(1024*1024*1024):.2f} GB"

    def _extract_transferred_files(self, stdout: str) -> List[str]:
        """Extract list of transferred files from rsync output."""
        transferred_files = []

        # Split output into lines and process
        for line in stdout.splitlines():
            # Skip common status message lines
            if any(pattern in line for pattern in
                ['building file list', 'files to consider', 'sent', 'total size', 'bytes/sec']):
                continue

            # Most rsync outputs show files on their own line before any stats
            # Try to extract just the filename (ignoring paths and stats)
            parts = line.strip().split()
            if not parts:
                continue

            # Files typically appear at the start of lines, before transfer stats
            filename = parts[0]

            # Skip purely numeric entries and other likely non-filenames
            if (filename.isdigit() or
                filename in ['sending', 'sent', 'total', 'building'] or
                '%' in filename):
                continue

            # Get base filename and add if it looks valid
            base_name = os.path.basename(filename)
            if base_name and not base_name.isdigit():
                transferred_files.append(base_name)

        # Make sure our list is unique - no duplicates
        transferred_files = list(dict.fromkeys(transferred_files))

        return transferred_files

    def _extract_file_info(self, stdout: str):
        """Extract file information from rsync output."""
        stats_lines = []
        total_bytes = 0

        # Try to extract file count from rsync output
        file_count_match = re.search(r'(\d+) files? to consider', stdout)
        file_count = file_count_match.group(1) if file_count_match else "0"

        # Extract bytes sent/received
        bytes_match = re.search(r'sent ([\d,]+) bytes\s+received ([\d,]+) bytes', stdout)
        if bytes_match:
            sent = bytes_match.group(1).replace(',', '')
            received = bytes_match.group(2).replace(',', '')
            try:
                total_bytes = int(sent) + int(received)
            except ValueError:
                # In case of parsing issues
                total_bytes = 0

        # Calculate time taken
        time_taken = (datetime.datetime.now() - self._last_sync_time).total_seconds()
        time_str = f"{time_taken:.2f}s"

        # Extract transferred files
        transferred_files = self._extract_transferred_files(stdout)

        # Format transferred files with truncation if needed
        files_summary = ""
        if transferred_files:
            if len(transferred_files) <= 3:
                files_summary = ", ".join(transferred_files)
            else:
                files_summary = f"{transferred_files[0]}, {transferred_files[1]}, ... +{len(transferred_files)-2} more"

        # Format total bytes in human readable format
        size_str = self._format_size(total_bytes)

        # Build stats lines
        stats_lines.append(f"Files: {file_count}, Size: {size_str}, Time: {time_str}")
        if files_summary:
            stats_lines.append(f"Transferred: {files_summary}")

        return stats_lines

    def _log_rsync_output(self, stdout: str):
        """Log the rsync stdout in a readable format."""
        if not self._logger:
            return

        self._log("debug", "===== RSYNC OUTPUT START =====")
        for line in stdout.splitlines():
            if line.strip():
                self._log("debug", f"RSYNC: {line}")
        self._log("debug", "===== RSYNC OUTPUT END =====")

    def _perform_sync(self, path: Path):
        """Perform the actual sync operation."""
        cmd = self._build_rsync_command(path)
        self._sync_count += 1
        self._last_sync_time = datetime.datetime.now()

        # Always log the exact command at DEBUG level
        command_str = ' '.join(cmd)
        self._log("debug", f"COMMAND: {command_str}")

        status_text = Text()
        status_text.append("ESync: ", style="bold cyan")
        status_text.append(f"Sync #{self._sync_count} ", style="yellow")
        status_text.append("in progress ", style="italic")
        status_text.append(f"[{self._last_sync_time.strftime('%Y-%m-%d %H:%M:%S')}]", style="dim")
        self._update_status(status_text)

        self._log("info", f"Starting sync #{self._sync_count} from {path}")

        # Always capture stdout to extract info, even without logging
        self._current_sync = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True
        )

        try:
            stdout, stderr = self._current_sync.communicate()

            # Log the full rsync output at DEBUG level
            if stdout:
                self._log_rsync_output(stdout)

            if self._current_sync.returncode != 0:
                if stderr:
                    self._log("error", f"Rsync stderr: {stderr}")

                self._last_sync_status = False
                status_text = Text()
                status_text.append("ESync: ", style="bold cyan")
                status_text.append(f"Sync #{self._sync_count} ", style="yellow")
                status_text.append("failed ", style="bold red")
                status_text.append(f"[{datetime.datetime.now().strftime('%Y-%m-%d %H:%M:%S')}]", style="dim")
                status_text.append("\n", style="default")
                status_text.append(f"Error: {stderr.strip() if stderr else f'Exit code {self._current_sync.returncode}'}", style="red")
                self._update_status(status_text)

                error_msg = f"Sync failed with code {self._current_sync.returncode}"
                self._log("error", error_msg)
                if not self._quiet:
                    console.print(f"[bold red]✗[/] {error_msg}", highlight=False)

                raise subprocess.CalledProcessError(
                    self._current_sync.returncode, cmd, stdout, stderr
                )
            else:
                # Always extract stats even without logging
                stats_lines = self._extract_file_info(stdout)

                self._last_sync_status = True
                status_text = Text()
                status_text.append("ESync: ", style="bold cyan")
                status_text.append(f"Sync #{self._sync_count} ", style="yellow")
                status_text.append("completed ", style="bold green")
                status_text.append(f"[{datetime.datetime.now().strftime('%Y-%m-%d %H:%M:%S')}]", style="dim")

                for stats_line in stats_lines:
                    status_text.append("\n", style="default")
                    status_text.append(stats_line, style="green dim")

                self._update_status(status_text)

                self._log("info", f"Sync #{self._sync_count} completed successfully")
                if not self._quiet:
                    console.print(f"[bold green]✓[/] Sync #{self._sync_count} completed successfully", highlight=False)
        finally:
            self._current_sync = None

    def _parse_remote_string(self, remote_str: str) -> tuple:
        """
        Parse a remote string into username, host, and path components.
        Format: [user@]host:path
        Returns: (username, host, path)
        """
        match = re.match(r'^(?:([^@]+)@)?([^:]+):(.+)$', remote_str)
        if match:
            return match.groups()
        return None, None, remote_str

    def _is_remote_path(self, path_str: str) -> bool:
        """
        Determine if a string represents a remote path.
        A remote path is in the format [user@]host:path.
        """
        # Avoid treating Windows paths (C:) as remote
        if len(path_str) >= 2 and path_str[1] == ':' and path_str[0].isalpha():
            return False
        # Simple regex to match remote path format
        return bool(re.match(r'^(?:[^@]+@)?[^/:]+:.+$', path_str))


    def _build_rsync_command(self, source_path: Path) -> list[str]:
        """Build rsync command for local or remote sync."""
        cmd = [
            "rsync",
            "--recursive",  # recursive
            "--times",      # preserve times
            "--progress",   # progress for parsing
            # "--verbose",    # verbose for parsing
            # "--links",      # copy symlinks as symlinks
            # "--copy-links", # transform symlink into referent file/dir
            "--copy-unsafe-links", # only "unsafe" symlinks are transformed
        ]

        # Add backup if enabled
        if hasattr(self._config, 'backup_enabled') and self._config.backup_enabled:
            cmd.append("--backup")
            backup_dir = getattr(self._config, 'backup_dir', '.rsync_backup')
            cmd.append(f"--backup-dir={backup_dir}")

        # Add other rsync options if configured
        if hasattr(self._config, 'compress') and self._config.compress:
            cmd.append("--compress")
        if hasattr(self._config, 'human_readable') and self._config.human_readable:
            cmd.append("--human-readable")
        if hasattr(self._config, 'verbose') and self._config.verbose:
            cmd.append("--verbose")
        # Todo this is where we add standard rsync commands


        # Add ignore patterns
        for pattern in self._config.ignores:
            # Remove any quotes and brackets from the input
            clean_pattern = pattern.strip('"[]\'')
            # Handle **/ pattern (recursive)
            if clean_pattern.startswith('**/'):
                clean_pattern = clean_pattern[3:]  # Remove **/ prefix
            cmd.extend(["--exclude", clean_pattern])

        # Ensure we have absolute paths for the source
        source = f"{source_path.absolute()}/"

        # Get target as string
        target_str = str(self._config.target)
        # Determine if target is a remote path
        is_remote = self._is_remote_path(target_str)


        if self._config.is_remote():
            # For remote sync via SSH config object
            ssh = self._config.ssh
            if ssh.user:
                remote = f"{ssh.user}@{ssh.host}:{self._config.target}"
            else:
                remote = f"{ssh.host}:{self._config.target}"
            cmd.append(source)
            cmd.append(remote)

        elif is_remote:
            # For direct remote specification (host:path)
            # Use the target string directly without any modification
            cmd.append(source)
            cmd.append(target_str)

            # Log for debugging
            self._log("debug", f"Remote path detected: '{target_str}'")

        else:
            # For local sync
            try:
                target = Path(target_str).expanduser()
                target.mkdir(parents=True, exist_ok=True)
            except Exception as e:
                self._log("error", f"Error creating target directory: {e}")
                raise

            cmd.append(source)
            cmd.append(str(target) + '/')

        # Log the final command for debugging
        self._log("debug", f"Final rsync command: {' '.join(cmd)}")

        return cmd
