# esync

A basic file sync tool based on watchdog/watchman and rsync.

Tested for rsync version 3.4.1 and python >=3.9.
For more information on rsync and options, visit the [manual page.](https://linux.die.net/man/1/rsync)


## Installation

With pip
```bash
git clone https://github.com/eloualic/esync.git
cd esync
pip install -e .
```

## Usage

### Configuration

To create a configuration file, run the following command:
```bash
esync init --help
```

### Basic command

Local sync
```bash
esync sync -l test-sync/source -r test-sync/target
```

Remote sync
```bash
esync sync -l test-sync/source -r user@remote:/path/to/target
esync sync -l test-sync/source -r remote:/path/to/target # or based on ssh config
```


## Future

- Two step authentication
- Statistics
- General option for rsync
