# Stream Guy

A Windows desktop application for Twitch streamers that displays interactive chat popups, GIF reactions, and more. Built with [Gio](https://gioui.org/) for a native Go GUI experience.

## Features

- **Chat Popups** - Floating windows displaying chat messages with Twitch emotes and badges
- **Keyword GIF Reactions** - Trigger GIF popups when viewers type specific keywords
- **TTS (Text-to-Speech)** - Channel point redemptions can trigger spoken messages
- **Photo Popups** - Display viewer-submitted images via channel point redemptions
- **Drawing Overlay** - Transparent overlay for on-screen drawing (toggle with double-Shift)
- **Raspberry Pi Integration** - Send accepted images to a Pi display
- **PostgreSQL Support** - Optional database integration

## Requirements

- Windows 10+
- Go 1.25+
- [Streamer.bot](https://streamer.bot/) running with WebSocket server enabled

## Configuration

### Environment Variables

Create a `.env` file in the project root:

```env
STREAMERBOT_HOST=127.0.0.1
STREAMERBOT_PORT=8080
PUBLIC_PI=http://your-pi-address:port  # Optional
POSTGRES_URL=postgres://user:pass@host:5432/db  # Optional
```

### config.json

Define keyword-to-GIF mappings:

```json
{
  "keywords": {
    "Joel": "Joel",
    "Classic": "Classic"
  }
}
```

Place corresponding GIF files in `assets/gifs/` (e.g., `assets/gifs/Joel.gif`).

## Building

```bash
go build -o stream-guy.exe .
```

## Usage

1. Start Streamer.bot with WebSocket server enabled
2. Run `stream-guy.exe`
3. The control panel window shows connection status and active popup count
4. Use the control panel buttons to pause popups or clear all windows

## Control Panel

- **Streamer.bot status** - Shows WebSocket connection state
- **Active windows** - Number of open popup windows
- **Performance** - Memory usage and goroutine count
- **Clear All** - Close all popup windows
- **Pause/Resume Popups** - Toggle popup creation
- **Clear Images** - Clear images on connected Pi display

## Drawing Overlay

Press **Control+Shift** to toggle drawing mode. When enabled, a transparent overlay covers the screen for on-stream drawing.

### Controls

| Input             | Action               |
|-------------------|----------------------|
| Left-click + drag | Draw stroke          |
| Right-click       | Undo last stroke     |
| Middle-click      | Clear all strokes    |
| Scroll wheel      | Cycle through colors |
| Control+Shift     | Toggle drawing mode  |

## License

See [LICENSE](LICENSE) for details.
