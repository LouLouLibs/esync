from pathlib import Path
from typing import Optional, Union
import typer
from enum import Enum
from rich.console import Console
from rich.table import Table
from rich.live import Live

from .sync_manager import SyncManager
from .watchdog_watcher import WatchdogWatcher
from .watchman_watcher import WatchmanWatcher
from .watcher_base import WatcherBase
from .config import (
    load_config,
    find_config_file,
    ESyncConfig,
    SyncConfig,
    SSHConfig,
    get_default_config,
    create_config_for_paths
)

app = typer.Typer(
    name="esync",
    help="File synchronization tool with watchdog/watchman support",
    add_completion=False,
)

verbose_help_init = """
esync - File synchronization tool

Basic Usage:
  esync init                 # Initialize a new configuration
  esync init -c esync.toml   # Create a new configuration file
"""

verbose_help_sync = """
esync - File synchronization tool

Basic Usage:
  esync sync                   # Start syncing with configuration file
  esync sync -c esync.toml     # Use specific configuration file
  esync sync -l ./local -r ./remote   # Override paths in config

Quick Sync:
  esync sync --quick -l ./local -r ./remote  # Quick sync with default settings
  esync sync -q -l ./local -r user@host:/path  # Quick sync to remote SSH

Logging:
  esync sync --log sync.log    # Log operations to file
  esync sync -q -l ./local -r ./remote --log sync.log  # Quick sync with logging

Output Control:
  esync sync                  # Default: clean minimal output with status panel
  esync sync --no-quiet       # Show more console output
  esync sync -v               # Enable verbose mode with detailed output
  esync sync --log sync.log   # Log operations to file
  esync sync -v --log sync.log  # Detailed logging to file
"""



console = Console()

class WatcherType(str, Enum):
    WATCHDOG = "watchdog"
    WATCHMAN = "watchman"

def create_watcher(
    watcher_type: WatcherType,
    source_path: Path,
    sync_manager: SyncManager
) -> Union[WatchmanWatcher, WatchdogWatcher]:
    """Create appropriate watcher based on type."""
    if watcher_type == WatcherType.WATCHDOG:
        return WatchdogWatcher(source_path, sync_manager)
    return WatchmanWatcher(source_path, sync_manager)
# --------------------------------------------------------------------------------------------------


# --------------------------------------------------------------------------------------------------
def display_config(config: ESyncConfig) -> None:
    """Display the current configuration."""
    table = Table(title="Current Configuration")
    table.add_column("Section", style="cyan")
    table.add_column("Setting", style="magenta")
    table.add_column("Value", style="green")

    sync_data = config.model_dump().get('sync', {})

    # Local configuration
    local_config = sync_data.get("local", {})
    table.add_row("Local", "path", str(local_config.get("path", "Not set")))
    table.add_row("Local", "interval", str(local_config.get("interval", 1)))

    # Remote configuration
    remote_config = sync_data.get("remote", {})
    table.add_row("Remote", "path", str(remote_config.get("path", "Not set")))
    if ssh := remote_config.get("ssh"):
        table.add_row("Remote", "ssh.host", ssh.get("host", ""))
        table.add_row("Remote", "ssh.user", ssh.get("user", ""))
        table.add_row("Remote", "ssh.port", str(ssh.get("port", 22)))

    # ESync settings
    esync_settings = config.settings.esync
    table.add_row("ESync", "watcher", esync_settings.watcher)
    if esync_settings.ignore:
        table.add_row("ESync", "ignore", "\n".join(esync_settings.ignore))

    # Rsync settings
    rsync_settings = config.settings.rsync
    for key, value in rsync_settings.model_dump().items():
        if isinstance(value, list):
            value = "\n".join(value)
        elif isinstance(value, bool):
            value = "✓" if value else "✗"
        table.add_row("Rsync", key, str(value))

    console.print(table)

@app.callback()
def main():
    """File synchronization tool with watchdog/watchman support."""
    pass
# --------------------------------------------------------------------------------------------------


# --------------------------------------------------------------------------------------------------
@app.command()
def sync(
    ctx: typer.Context,
    config_file: Optional[Path] = typer.Option(
        None,
        "--config",
        "-c",
        help="Path to TOML config file"
    ),
    local: Optional[str] = typer.Option(
        None,
        "--local",
        "-l",
        help="Local path to sync from"
    ),
    remote: Optional[str] = typer.Option(
        None,
        "--remote",
        "-r",
        help="Remote path to sync to"
    ),
    watcher: Optional[WatcherType] = typer.Option(
        None,
        "--watcher",
        "-w",
        help="Override watcher type"
    ),
    quick: bool = typer.Option(
        False,
        "--quick",
        "-q",
        help="Quick sync with default settings"
    ),
    log_file: Optional[Path] = typer.Option(
        None,
        "--log",
        help="Path to log file"
    ),
    quiet: bool = typer.Option(
        True,
        "--quiet/--no-quiet",
        help="Reduce console output (default: quiet)"
    ),
    verbose: bool = typer.Option(
        False,
        "--verbose",
        "-v",
        help="Enable verbose output with detailed logging"
    ),
    help_override: bool = typer.Option(False, "--help", is_eager=True, help="Show help message"),
):
    """Start the file synchronization service."""
    if help_override:
        console.print(ctx.get_help(), style="bold")
        if verbose:
            console.print(verbose_help_sync, style="italic")
        raise typer.Exit()

    try:
        # Handle quick sync option
        if quick:
            if not local or not remote:
                console.print("[red]Both local and remote paths are required with --quick option[/]")
                raise typer.Exit(1)

            # Create quick configuration
            config = create_config_for_paths(local, remote, watcher.value if watcher else None)
            if not quiet:
                console.print("[bold blue]Using quick sync configuration[/]")

                # Display effective configuration
                console.print("\n[bold]Quick Sync Configuration:[/]")
                display_config(config)
        else:
            # Find and load config file (original flow)
            config_path = config_file or find_config_file()
            if not config_path:
                console.print("[red]No configuration file found![/]")
                console.print("\t[green]Try running 'esync init' to create one.")
                console.print("\tOr use 'esync sync --quick -l LOCAL -r REMOTE' for quick syncing.[/]")
                raise typer.Exit(1)

            # Show which config file we're using
            if not quiet:
                console.print(f"[bold blue]Loading configuration from:[/] {config_path.resolve()}")

            try:
                config = load_config(config_path)
            except Exception as e:
                console.print(f"[red]Failed to load config: {e}[/]")
                raise typer.Exit(1)

            sync_data = config.model_dump().get('sync', {})

            # Validate required sections
            if 'local' not in sync_data or 'remote' not in sync_data:
                console.print("[red]Invalid configuration: 'sync.local' and 'sync.remote' sections required[/]")
                raise typer.Exit(1)

            # Override config with CLI options
            if local:
                sync_data['local']['path'] = local
            if remote:
                sync_data['remote']['path'] = remote
            if watcher:
                config.settings.esync.watcher = watcher.value

            # Display effective configuration
            if not quiet:
                console.print("\n[bold]Effective Configuration:[/]")
                display_config(config)

        # Get sync data from config
        sync_data = config.model_dump().get('sync', {})

        # Prepare paths
        local_path = Path(sync_data['local']['path']).expanduser().resolve()
        local_path.mkdir(parents=True, exist_ok=True)

        # Create sync configuration
        remote_config = sync_data['remote']
        rsync_settings = config.settings.rsync

        if "ssh" in remote_config:
            sync_config = SyncConfig(
                target=remote_config["path"],
                ssh=SSHConfig(**remote_config["ssh"]),
                ignores=rsync_settings.ignore + config.settings.esync.ignore,
                backup_enabled=rsync_settings.backup_enabled,
                backup_dir=rsync_settings.backup_dir,
                compress=rsync_settings.compress,
                human_readable=rsync_settings.human_readable
            )
        else:
            remote_path = Path(remote_config["path"]).expanduser().resolve()
            remote_path.mkdir(parents=True, exist_ok=True)
            sync_config = SyncConfig(
                target=remote_path,
                ignores=rsync_settings.ignore + config.settings.esync.ignore,
                backup_enabled=rsync_settings.backup_enabled,
                backup_dir=rsync_settings.backup_dir,
                compress=rsync_settings.compress,
                human_readable=rsync_settings.human_readable
            )

        # Initialize sync manager and watcher
        log_file_path = str(log_file) if log_file else None

        # Create sync manager and watcher
        sync_manager = SyncManager(sync_config, log_file_path)
        # Apply quiet/verbose settings
        sync_manager._quiet = quiet
        sync_manager._verbose = verbose
        watcher = create_watcher(
            WatcherType(config.settings.esync.watcher),
            local_path,
            sync_manager
        )

        if not quiet:
            console.print(f"\nStarting {config.settings.esync.watcher} watcher...")
            if log_file:
                console.print(f"[bold blue]Logging to:[/] {log_file}")

        # Start with Live display
        try:
            watcher.start()

            if not quiet:
                console.print("[bold green]Watcher started successfully. Press Ctrl+C to stop.[/]")

            with Live(sync_manager.status_panel, refresh_per_second=4, console=console) as live:
                while True:
                    live.update(sync_manager.status_panel)
                    import time
                    time.sleep(0.5)

        except KeyboardInterrupt:
            if not quiet:
                console.print("\nStopping watcher...")
            watcher.stop()
            sync_manager.stop()
            if not quiet:
                console.print("[bold green]Watcher stopped successfully.[/]")

    except Exception as e:
        console.print(f"[red]Error: {str(e)}[/]")
        raise typer.Exit(1)
# --------------------------------------------------------------------------------------------------



# --------------------------------------------------------------------------------------------------
@app.command()
def init(
    ctx: typer.Context,
    config_file: Path = typer.Option(
        Path("esync.toml"), "--config", "-c", help="Path to create config file"
    ),
    verbose: bool = typer.Option(False, "--verbose", help="Enable verbose output"),
    help_override: bool = typer.Option(False, "--help", is_eager=True, help="Show help message"),
    ):
    """Initialize a new configuration file."""
    if help_override:
        console.print(ctx.get_help(), style="bold")
        if verbose:
            console.print(verbose_help_init, style="italic")
        raise typer.Exit()

    if config_file.exists():
        overwrite = typer.confirm(
            f"Config file {config_file} already exists. Overwrite?",
            abort=True
        )

    # Get default config from the central location
    default_config = get_default_config()

    # Write config to file
    import tomli_w
    with open(config_file, 'wb') as f:
        tomli_w.dump(default_config, f)

    console.print(f"[green]Created config file: {config_file}[/]")
# --------------------------------------------------------------------------------------------------


# --------------------------------------------------------------------------------------------------
if __name__ == "__main__":
    app()
