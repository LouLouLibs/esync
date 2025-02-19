from pathlib import Path
from typing import Optional, List, Dict, Any, Union
from pydantic import BaseModel, Field
import tomli
from rich.console import Console

console = Console()

class SSHConfig(BaseModel):
    host: str
    user: Optional[str] = None
    port: int = 22

class SyncConfig(BaseModel):
    target: Union[Path, str]
    ssh: Optional[SSHConfig] = None
    ignores: List[str] = Field(default_factory=list)

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
