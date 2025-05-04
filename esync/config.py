from pathlib import Path
from typing import Optional, List, Dict, Any, Union
from pydantic import BaseModel, Field
import re
import tomli
from rich.console import Console

console = Console()

class SSHConfig(BaseModel):
    host: str
    user: Optional[str] = None
    port: int = 22
    allow_password_auth: bool = True
    identity_file: Optional[str] = None
    interactive_auth: bool = True  # Enable interactive authentication prompts

class SyncConfig(BaseModel):
    target: Union[Path, str]
    ssh: Optional[SSHConfig] = None
    ignores: List[str] = Field(default_factory=list)
    backup_enabled: bool = False
    backup_dir: str = ".rsync_backup"
    compress: bool = True
    human_readable: bool = True
    verbose: bool = False


    def is_remote(self) -> bool:
        """Check if this is a remote sync configuration."""
        return self.ssh is not None

    def get_target_path(self) -> Path:
        """Get the target path as a Path object."""
        if isinstance(self.target, str):
            return Path(self.target).expanduser()
        return self.target




class RemoteConfig(BaseModel):
    path: Union[Path, str]
    ssh: Optional[SSHConfig] = None

class RsyncSettings(BaseModel):
    backup_enabled: bool = True
    backup_dir: str = ".rsync_backup"
    compression: bool = True
    verbose: bool = False
    archive: bool = True
    compress: bool = True
    human_readable: bool = True
    progress: bool = True
    ignore: List[str] = Field(default_factory=list)

class ESyncSettings(BaseModel):
    watcher: str = "watchdog"
    ignore: List[str] = Field(default_factory=list)

class Settings(BaseModel):
    esync: ESyncSettings = Field(default_factory=ESyncSettings)
    rsync: RsyncSettings = Field(default_factory=RsyncSettings)

class ESyncConfig(BaseModel):
    sync: Dict[str, Any] = Field(default_factory=dict)
    settings: Settings = Field(default_factory=Settings)

def get_default_config() -> Dict[str, Any]:
    """Get the default configuration."""
    return {
        "sync": {
            "local": {
                "path": "./local",
                "interval": 1
            },
            "remote": {
                "path": "./remote"
            }
        },
        "settings": {
            "esync": {
                "watcher": "watchdog",
                "ignore": [
                    "*.log",
                    "*.tmp",
                    ".env"
                ]
            },
            "rsync": {
                "backup_enabled": True,
                "backup_dir": ".rsync_backup",
                "compression": True,
                "verbose": False,
                "archive": True,
                "compress": True,
                "human_readable": True,
                "progress": True,
                "ignore": [
                    "*.swp",
                    ".git/",
                    "node_modules/",
                    "**/__pycache__/",
                ]
            }
        }
    }

def create_config_for_paths(local_path: str, remote_path: str, watcher_type: Optional[str] = None) -> ESyncConfig:
    """Create a configuration with specific paths."""
    # Start with default config
    config_dict = get_default_config()

    # Update paths
    config_dict["sync"]["local"]["path"] = local_path

    # Set watcher if provided
    if watcher_type:
        config_dict["settings"]["esync"]["watcher"] = watcher_type

    # Handle SSH configuration if needed -> use the function ... that is defined above like is remote path
    # check if config is remote
    is_remote_ssh = False # check if we have to deal with ssh or not
    if ":" in remote_path:
        if not ( len(remote_path) >= 2 and remote_path[1] == ':' and remote_path[0].isalpha() ):
            is_remote_ssh = True

    # now we split the remote path between ssh case and non ssh case
    if is_remote_ssh:
        # Extract user, host, and path
        user_host, path = remote_path.split(":", 1)
        if "@" in user_host:
            user, host = user_host.split("@", 1)
            config_dict["sync"]["remote"] = {
                "path": path,
                "ssh": {
                    "host": host,
                    "user": user,
                    "port": 22
                }
            }
        else:
            # No user specified
            config_dict["sync"]["remote"] = {
                "path": path,
                "ssh": {
                    "host": user_host,
                    "port": 22
                }
            }
    else:
        # Local path
        config_dict["sync"]["remote"]["path"] = remote_path

    return ESyncConfig(**config_dict)

def load_config(config_path: Path) -> ESyncConfig:
    """Load and validate TOML configuration file."""
    try:
        with open(config_path, "rb") as f:
            config_data = tomli.load(f)
            return ESyncConfig(**config_data)
    except FileNotFoundError:
        console.print(f"[yellow]Config file not found: {config_path}[/]")
        raise
    except tomli.TOMLDecodeError as e:
        console.print(f"[red]Error parsing TOML file: {e}[/]")
        raise
    except Exception as e:
        console.print(f"[red]Error loading config: {e}[/]")
        raise

def get_default_config_paths() -> List[Path]:
    """Return list of default config file locations in order of precedence."""
    return [
        Path.cwd() / "esync.toml",  # Current directory
        Path.home() / ".config" / "esync" / "config.toml",  # User config directory
        Path("/etc/esync/config.toml"),  # System-wide config
    ]

def find_config_file() -> Optional[Path]:
    """Find the first available config file from default locations."""
    for path in get_default_config_paths():
        if path.is_file():
            return path
    return None
