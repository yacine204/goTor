# goTor 

conccurent bittorrent client, supports multi peer downloading and auto reconnection.

## Installation

### Requisitiries 

- Go 1.21 or higher

## Build from source

```bash
clone https://github.com/yacine204/goTor.git
cd goTor
go build -o torrent main.go
```
### Usage 

```bash 
./torrent <torrent-file> <download-path>
```

### Logs

logs are saved in **<download-path>/logs**

### Project Structure
 
| Path | Purpose |
|------|---------|
| `main.go` | Cli entrypoint |
| `core/metadata` | Shared data types (TorrentMetaData, Info, FileItem) |
| `utils/trackers.go` | Tracker communication (UDP/HTTP)|
| `utils/peer.go` | Peer connection and handshake, main download loop,|
| `utils/encoding.go` | Bencode encode/decode |

## Architecture 

```
1. Parse .torrent file -> Extract metadata
2. Connect to trackers -> Get peer list
3. Connect to peers -> Handshake
4. Exchange bitfield -> Know what peers have
5. Send Interested -> Request pieces
6. Receive Unchoke -> Start downloading
7. Request blocks -> 16KB at a time
8. Receive blocks -> Save to disk
9. Piece complete -> Verify SHA-1
10. Send Have -> Tell peers
11. Repeat -> Download next piece
12. Verify if all pieces are downloaded -> end loop
```

## Technical Details 

### Piece Request Strategy
 - Rarest First Prioritizes pieces with the fewest copies in the swarm
 - Block Size 16KB (16384 bytes) for efficient transfers
 - Concurrent Requests - Up to 5 blocks per peer

### Error Handling
 - Connection Errors - Automatic reconnection with new peers
 - Choke Handling - Seamless peer switching when choked
 - Timeout Recovery - 120-second stall detection
 - Verification Failures - Automatic re-request of corrupted pieces


## Known Issues
 - UDP Trackers: Some UDP trackers may be unreliable
 - NAT/Firewall: Requires open port for incoming connections
 - Large Torrents: May require increased file descriptor limits


## License 
MIT License, see [license](LICENSE)