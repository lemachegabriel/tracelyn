# Tracelyn

**Tracelyn** is a CLI tool that records terminal sessions to clean plain text files. It creates a transparent sub-shell using PTY (pseudo-terminal) that captures all commands and outputs without interfering with your workflow.

## Features

- **Transparent Recording**: Creates an invisible sub-shell that records everything
- **Clean Output**: Uses virtual terminal emulation to extract clean text (no ANSI codes)
- **Multiple Sessions**: Support for concurrent recording sessions
- **Session Management**: Track, list, stop, and remove sessions easily
- **Merge Sessions**: Combine multiple session recordings chronologically
- **Zero Configuration**: Works out of the box with your existing shell setup

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap lemachegabriel/tap
brew install tracelyn
```

### APT (Debian/Ubuntu)

```bash
# Add repository (first time only)
echo "deb [trusted=yes] https://apt.fury.io/lemachegabriel/ /" | sudo tee /etc/apt/sources.list.d/tracelyn.list
sudo apt update
sudo apt install tracelyn
```

### Manual Installation

Download the latest release from the [releases page](https://github.com/lemachegabriel/tracelyn/releases) and extract it to a directory in your `$PATH`.

### From Source

```bash
git clone https://github.com/lemachegabriel/tracelyn.git
cd tracelyn
go build -o tracelyn .
sudo mv tracelyn /usr/local/bin/
```

## Usage

### Start Recording

Start a new terminal session recording:

```bash
tracelyn record
```

This will start a transparent sub-shell. All your commands and outputs will be recorded to a plain text file in `~/.tracelyn/sessions/`.

### Stop Recording

Exit the recording session by typing `exit` or pressing `Ctrl+D`, or stop it from another terminal:

```bash
tracelyn stop [session-id]
```

### List Sessions

View all sessions (active and completed):

```bash
tracelyn list
```

### Remove Sessions

Remove a completed session:

```bash
tracelyn remove <session-id>
```

Remove all completed sessions:

```bash
tracelyn remove --all
```

### Merge Sessions

Merge multiple session recordings chronologically:

```bash
tracelyn merge <id1> <id2> <id3> -o output.txt
```

## How It Works

Tracelyn creates a transparent sub-shell that:

1. Uses your default shell (`$SHELL`) with your configuration
2. Inherits your environment variables and working directory
3. Captures all I/O through a PTY (pseudo-terminal)
4. Processes ANSI escape sequences using a virtual terminal emulator
5. Extracts clean, readable text without control codes
6. Saves everything to plain text files

## Session Files

- **Location**: `~/.tracelyn/sessions/`
- **Format**: `session_YYYYMMDD_HHMMSS.txt`
- **Registry**: `~/.tracelyn/sessions.json`

## Examples

### Record a build process

```bash
tracelyn record
make build
make test
exit  # Stop recording
```

### Record and merge multiple sessions

```bash
# Terminal 1
tracelyn record  # Creates session 1
# ... do some work ...
exit

# Terminal 2
tracelyn record  # Creates session 2
# ... do more work ...
exit

# Merge sessions
tracelyn merge 1 2 -o complete-workflow.txt
```

## Development

### Requirements

- Go 1.25.1 or higher

### Build

```bash
go build -o tracelyn .
```

### Run Tests

```bash
go test ./...
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License - see [LICENSE](LICENSE) for details.

## Author

Gabriel Lemache

## Links

- [GitHub Repository](https://github.com/lemachegabriel/tracelyn)
- [Issue Tracker](https://github.com/lemachegabriel/tracelyn/issues)
- [Releases](https://github.com/lemachegabriel/tracelyn/releases)
