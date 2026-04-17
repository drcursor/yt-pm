# YouTube (TUI) Playlist Manager — `yt-pm`

> **⚠ Experimental software — use at your own risk.**
>
> `yt-pm` relies on scraping and reverse-engineered internal YouTube APIs that are **not officially supported by Google**. These interfaces can change or break without notice. This project has no affiliation with YouTube or Google. Do not use it for anything critical or at scale — it may stop working at any time and could violate YouTube's Terms of Service.

A terminal UI for managing YouTube playlists — move, copy, export, clear, and inspect videos without the YouTube Data API.

**Who is this for?**
`yt-pm` is built for people who have spent years saving videos to YouTube playlists and now face hundreds or thousands of entries that need sorting, pruning, or reorganizing. If you have a "Watch Later" queue that's grown out of control, playlists full of deleted or private videos, or a library you want to restructure from scratch — this tool gives you a fast, keyboard-driven way to do it without clicking through YouTube's web UI one video at a time.

## Features

- **List** your YouTube playlists
- **Browse** a playlist's videos and act on individual or selected videos
- **Move** videos from one playlist to another
- **Copy** videos from one playlist to another (with optional skip-duplicates)
- **Export** a playlist to a structured JSON file (title, channel, duration, video ID)
- **Clear** all videos from a playlist
- **Delete** a playlist entirely (requires typing the playlist name to confirm)
- **Find broken/unlisted videos** — scan a playlist for private, deleted, or unlisted entries and optionally remove them
- **Stats** — total duration, average, longest/shortest video, and unique channel count for any playlist
- **Open in browser** — press `o` on any video to open it in your default browser

All destructive actions show a full per-video plan and require explicit confirmation before anything is changed.

## Authentication

`yt-pm` uses your existing YouTube browser session — no API key required. On first run it will ask you to paste your YouTube cookies in Netscape format. Your session is saved locally so you only need to do this once.

If your session expires mid-operation the app will pause and ask you to refresh your cookies before continuing.

## Installation

Requires Go 1.21+.

```bash
go install github.com/eduardobalsa/yt-pm@latest
```

Or build from source:

```bash
git clone https://github.com/eduardobalsa/yt-pm
cd yt-pm
go build -o yt-pm .
```

## Usage

```bash
yt-pm
```

Navigate with arrow keys or `j`/`k`. Press `q` or `Esc` to go back.

### Playlist list keys

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Navigate |
| `Enter` | Browse videos in playlist |
| `m` | Move all videos to another playlist |
| `c` | Copy all videos to another playlist |
| `e` | Export playlist to JSON |
| `x` | Clear all videos from playlist |
| `f` | Find broken/unlisted videos |
| `s` | Show playlist stats |
| `d` | Delete playlist |
| `q` | Quit |

### Video list keys

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Navigate |
| `Space` | Toggle selection |
| `a` | Select / deselect all |
| `c` | Copy selected (or current) video(s) |
| `m` | Move selected (or current) video(s) |
| `x` | Remove selected (or current) video(s) from playlist |
| `o` | Open video in browser |
| `Esc` | Back to playlist list |

### Skip duplicates

When copying or moving, the plan screen shows a `d` toggle to skip videos already present in the destination playlist.

### Export filename

The export plan screen pre-fills a filename based on the playlist name. Edit it before pressing `Enter` to confirm.

## Logs

Every run writes a timestamped log file (`yt_YYYYMMDD_HHMMSS.log`) to the working directory. Failures and skipped videos are recorded there without interrupting the rest of the operation.

## Disclaimer

AI Coding agents have been used in the creation of this project.


## License

GNU GENERAL PUBLIC LICENSE Version 3 — see [LICENSE](LICENSE).
