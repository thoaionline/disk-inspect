# disk-inspect

A Go terminal utility for finding what uses space beneath your current directory.
Open a folder, spot its largest entries, and drill down without leaving the keyboard.

```sh
go build -o disk-inspect .
./disk-inspect
./disk-inspect ~/Projects
```

Requires Go 1.22 or newer to build. Interactive mode needs a terminal at least
48 columns × 16 rows; 80 columns or more shows proportional usage bars.
The interface uses [Bubble Tea](https://github.com/charmbracelet/bubbletea).

```text
 DISK INSPECT  /  find where your space goes

 /home/you/Projects
 8.5 GiB allocated  ·  42,816 files  ·  2,019 folders
 Scan 1.2s  ·  sort: size ↓
 / filter entries

       SIZE   SHARE                NAME
›   5.2 GiB   61.2%  ━━━━━━━·····  web-app/
    2.1 GiB   24.7%  ━━━·········  archives/
    1.2 GiB   14.1%  ━━··········  datasets/
```

## Controls

| Key | Action |
| --- | --- |
| `↑` / `↓`, `k` / `j` | Move selection |
| `Enter`, `→`, `l` | Open selected directory |
| `Backspace`, `←`, `h` | Go to parent, up to the scan root |
| `~` | Return to scan root |
| `PgUp` / `PgDn`, `Ctrl+U` / `Ctrl+D` | Move a page |
| `g` / `G`, `Home` / `End` | First / last entry |
| `/` | Filter this folder by name, case-insensitively |
| `Enter` / `Esc` | Finish editing / clear filter |
| `s` | Cycle sort: size, name, file count |
| `a` | Toggle allocated / apparent size |
| `r` | Rescan the root, preserving the open folder if it still exists |
| `?` | Toggle help |
| `q`, `Ctrl+C` | Quit, including during a scan |

While editing a filter, letters (including `q`) enter text; `Ctrl+U` clears the
input. Press `Enter` to resume navigation. Refresh scans the original root.
The app only reads filesystem metadata and never deletes or changes your files.

## Size accounting

- **Allocated** is the default on Linux, macOS, and supported BSD systems. It uses
  the filesystem's allocated block count, so sparse files can use much less space
  than their apparent size. Other platforms fall back to apparent sizes.
- **Apparent** is the entry size reported by the filesystem. Start in this mode
  with `./disk-inspect --apparent [directory]`.
- Directory totals include their own metadata and every descendant. Percentages
  use the full current folder total, even when filtering. Hidden entries are included.
- Symlinks are counted as links and never followed inside the tree. An explicitly
  selected root symlink is resolved. Hard links are counted per pathname; shared,
  compressed, and reflinked storage is not deduplicated. Totals may therefore differ
  from `du`, free-space reports, or the space reclaimable by deleting an entry.
- Mount points below the root are traversed. Choose a narrower root to avoid
  scanning mounted volumes. Unreadable descendants show `!` and an error count;
  their totals are incomplete. An unreadable root is a scan failure.
- Scans are snapshots, so files changing during a scan can affect totals. The
  tree is held in memory (one node per entry), and navigation becomes available
  when scanning finishes. Cancellation is checked between filesystem operations;
  a stalled filesystem call can delay cancellation in JSON mode.

## JSON output

```sh
./disk-inspect --json . > usage.json
./disk-inspect --json --apparent ~/Projects
```

JSON includes root totals and an `entries` array of its immediate children, each
with recursive totals, file/directory counts, and error details. Sizes are integer
bytes. `allocated_supported` identifies whether allocated sizes are available.
Entries are sorted largest first by the chosen size mode. Flags go before the path.
Partial scans exit successfully and report `errors`; root failures exit with code 1.

## Development

```sh
make build
make check  # go vet and tests with the race detector
```

Tests cover recursive accounting, hidden files, symlinks, sparse files, unreadable
directories, cancellation, JSON output, filtering, navigation, and terminal sizing.
