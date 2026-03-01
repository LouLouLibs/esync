# ─────────────────────────────────────────────────────────────────────────────
# esync demo — watch local files and sync them with rsync, shown in a live TUI
#
# Usage:   vhs demo.tape
# Requires: go, rsync, vhs (https://github.com/charmbracelet/vhs)
# ─────────────────────────────────────────────────────────────────────────────

Output demo.gif
Require rsync

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 600
Set TypingSpeed 50ms
Set Theme "Catppuccin Mocha"
Set Padding 20
Set WindowBar Colorful
Set WindowBarSize 40

# ── Hidden setup ────────────────────────────────────────────────────────────
Hide

# Build esync from the current repo
Type "go build -o /tmp/esync-bin/esync . 2>/dev/null"
Enter
Sleep 3s
Type "export PATH=/tmp/esync-bin:$PATH"
Enter
Sleep 500ms

# Clean previous runs
Type "rm -rf /tmp/esync-demo /tmp/esync"
Enter
Sleep 500ms

# Create a demo project with source files from this repo
Type "mkdir -p /tmp/esync-demo/project/{cmd,internal/{tui,config,syncer,watcher}}"
Enter
Sleep 300ms
Type "cp main.go go.mod go.sum README.md /tmp/esync-demo/project/"
Enter
Sleep 300ms
Type "cp cmd/*.go /tmp/esync-demo/project/cmd/"
Enter
Sleep 300ms
Type "cp internal/tui/*.go /tmp/esync-demo/project/internal/tui/"
Enter
Sleep 300ms
Type "cp internal/config/*.go /tmp/esync-demo/project/internal/config/"
Enter
Sleep 300ms
Type "cp internal/syncer/*.go /tmp/esync-demo/project/internal/syncer/"
Enter
Sleep 300ms
Type "cp internal/watcher/*.go /tmp/esync-demo/project/internal/watcher/"
Enter
Sleep 300ms

# Write a minimal config file
Type `cat > /tmp/esync-demo/project/esync.toml << 'EOF'
[sync]
local  = "."
remote = "/tmp/esync"

[settings]
ignore = [".git", "node_modules", ".DS_Store", "build"]

[settings.rsync]
archive  = true
compress = false
EOF`
Enter
Sleep 500ms

Type "cd /tmp/esync-demo/project"
Enter
Sleep 300ms

Show

# ── Demo ────────────────────────────────────────────────────────────────────

# 1. Show the project
Type "ls"
Enter
Sleep 2s

# 2. Show the config file
Type "cat esync.toml"
Enter
Sleep 4s

# 3. Preview what will be synced
Type "esync check"
Enter
Sleep 5s

# 4. Queue background file edits (fire while the TUI is running)
Hide
Type '(sleep 5 && echo "// refactored handler" >> /tmp/esync-demo/project/cmd/sync.go && sleep 3 && printf "package cmd\n\nfunc Version() string { return \"1.0\" }\n" > /tmp/esync-demo/project/cmd/version.go && sleep 3 && echo "// updated styles" >> /tmp/esync-demo/project/internal/tui/styles.go) &'
Enter
Sleep 300ms
Show

# 5. Launch the TUI — file changes sync in real time
Type "esync sync"
Enter
Sleep 16s

# 6. Quit
Type "q"
Sleep 1s
