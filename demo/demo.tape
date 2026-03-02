# ─────────────────────────────────────────────────────────────────────────────
# esync demo — watch local files and sync them with rsync, shown in a live TUI
#
# Usage:   cd demo && vhs demo.tape
# Requires: esync, rsync, bat, vhs (https://github.com/charmbracelet/vhs)
# ─────────────────────────────────────────────────────────────────────────────

Output demo.gif
Require rsync
Require bat

Set FontSize 20
Set Width 1200
Set Height 600
Set Padding 20
Set Theme "GruvboxDarkHard"

Set TypingSpeed 100ms

Set Shell "bash"


# ── Hidden setup ────────────────────────────────────────────────────────────
Hide
    Type 'export PS1="> "'
    Enter
    # Clear the screen
    Type "clear"
    Enter
    Type "rm -rf /tmp/esync-demo"
    Enter
    # Sleep 100ms
Show


# ── Demo ──────────────────────────────────────────────────────────────────

# 1. Show the config
Type "bat esync.toml"
Enter
Sleep 4s

# 2. Show the project files
Type "ls"
Enter
Sleep 2s

# 3. Preview what will be synced
Type "esync check"
Sleep 500ms
Enter
Sleep 5s

# 4. Queue background file edits (fire while the TUI is running)
Hide
    Type '(sleep 5 && echo "// refactored" >> src/handler.go && sleep 3 && echo "// updated" >> config/settings.toml && sleep 3 && echo "// new file" > src/utils.go) &'
    Enter
    Sleep 300ms
Show

# 5. Launch the TUI — file changes sync in real time
Type "esync sync"
Sleep 500ms
Enter
Sleep 10s

# 6. Quit
Type "q"
Sleep 1s
