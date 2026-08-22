package utils

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"sync"
	"time"
	"torrent/core"
)

var peerID = "-TR0001-" + fmt.Sprintf("%012d", rand.N(1000000000000))

var failedPeers = make(map[string]bool)
var failedPeersMutex sync.Mutex

type PeerConn struct {
	Conn   net.Conn
	Peer   PeerNode
	PeerID string
}

type PeerState int

const (
	Choke PeerState = 0
	Unchoke PeerState = 1
	Interested PeerState = 2
	NotInterested PeerState = 3
	Have PeerState = 4
	Bitfield PeerState = 5
	Request PeerState = 6
	Piece PeerState = 7
)

func ConnectToPeer(peers []PeerNode) (PeerConn, error) {
	var wg sync.WaitGroup
	var mutex sync.Mutex

	var peerConn PeerConn
	var wonRace bool

	for _, peer := range peers {
		wg.Add(1)

		go func(p PeerNode) {
			defer wg.Done()

			conn, err := net.DialTimeout("tcp", net.JoinHostPort(p.Ip, p.Port), 5*time.Second)
			if err != nil {
				failedPeersMutex.Lock()
				failedPeers[p.Ip+":"+p.Port] = true
				failedPeersMutex.Unlock()
				fmt.Printf("error: %s\n", err)
				return
			}

			mutex.Lock()
			defer mutex.Unlock()

			if wonRace {
				conn.Close()
				return
			}

			wonRace = true
			peerConn = PeerConn{
				Peer: p,
				Conn: conn,
			}
		}(peer)
	}

	wg.Wait()

	if !wonRace {
		return PeerConn{}, fmt.Errorf("no peers could be connected")
	}
	return peerConn, nil
}


func ConnectAndHandshake(torrent *core.TorrentMetaData) (*PeerConn, error) {
	for{
		peers := ConnectTrackersAsync(torrent)

		var filteredPeers []PeerNode
		failedPeersMutex.Lock()

		for _, p := range peers{
			key := p.Ip + ":" + p.Port
			if !failedPeers[key]{
				filteredPeers = append(filteredPeers, p)
			}
		}

		if len(filteredPeers) == 0{
			fmt.Printf("all peers failed, resetting failed list\n")
			for k:=range failedPeers{
				delete(failedPeers, k)
			}
			filteredPeers = peers
		}

		failedPeersMutex.Unlock()

		peerConn, err := ConnectToPeer(filteredPeers)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to peer: %w", err)
		}

		_, ok, err := BitTorrentHandShake(&peerConn, torrent)
		if err != nil {
			peerConn.Conn.Close()
			failedPeersMutex.Lock()
			failedPeers[peerConn.Peer.Ip+":"+peerConn.Peer.Port] = true
			failedPeersMutex.Unlock()
			fmt.Printf("handshake error: %s, retrying\n", err)
			time.Sleep(1 * time.Second)
			continue
		}
		if !ok {
			peerConn.Conn.Close()
			failedPeersMutex.Lock()
			failedPeers[peerConn.Peer.Ip+":"+peerConn.Peer.Port] = true
			failedPeersMutex.Unlock()
			fmt.Printf("handshake rejected, retrying\n")
			time.Sleep(1 * time.Second)
			continue
		}

		return &peerConn, nil
	}
	
}

func BitTorrentHandShake(peer *PeerConn, torrent *core.TorrentMetaData) (*net.Conn, bool, error) {

	h := sha1.New()
	h.Write(torrent.RawInfoBytes)
	info_hash := h.Sum(nil)

	handshake := make([]byte, 68)
	handshake[0] = 19
	copy(handshake[1:20], []byte("BitTorrent protocol"))
	copy(handshake[28:48], info_hash)
	copy(handshake[48:68], []byte(peerID))

	n, err := peer.Conn.Write(handshake)
	if err != nil {
		return nil, false, fmt.Errorf("%s", err)
	}

	if n != 68 {
		return nil, false, fmt.Errorf("incomplete handshake write: wrote %d bytes", n)
	}

	response := make([]byte, 68)

	rn, err := io.ReadFull(peer.Conn, response)
	if err != nil {
		return nil, false, fmt.Errorf("handshake read failed after %d bytes: %w", rn, err)
	}

	headerCheck := int(response[0]) == 19
	protocolCheck := string(response[1:20]) == "BitTorrent protocol"
	infoHashCheck := bytes.Equal(response[28:48], info_hash)

	if headerCheck && protocolCheck && infoHashCheck {
		peer.PeerID = string(response[48:68])
		return &peer.Conn, true, nil
	}

	return nil, false, fmt.Errorf("handshake failed: header=%v, protocol=%v, info_hash=%v",
		headerCheck, protocolCheck, infoHashCheck)
}


func BitLoop(peerConn *PeerConn, torrent *core.TorrentMetaData, piecesCheck map[int]bool, downloadPath string) {
	receivedBlocks := make(map[int]map[int]bool)
	pendingRequests := make(map[int]map[int]bool)

	reconnect := func(reason string) bool {
		fmt.Printf("%s reconnecting to a new peer\n", reason)
		failedPeersMutex.Lock()
		failedPeers[peerConn.Peer.Ip+":"+peerConn.Peer.Port] = true
		failedPeersMutex.Unlock()
		peerConn.Conn.Close()

		newPeer, err := ConnectAndHandshake(torrent)
		if err != nil {
			fmt.Printf("failed to reconnect: %s\n", err)
			return false
		}

		*peerConn = *newPeer
		receivedBlocks = make(map[int]map[int]bool)
		pendingRequests = make(map[int]map[int]bool)
		return true
	}


	const stallTimeout = 120 * time.Second

	for {
		peerConn.Conn.SetReadDeadline(time.Now().Add(stallTimeout))

		messageLen := make([]byte, 4)
		_, err := io.ReadFull(peerConn.Conn, messageLen)
		if err != nil {
			if !reconnect(fmt.Sprintf("Connection error: %s", err)) {
				return
			}
			continue
		}

		length := binary.BigEndian.Uint32(messageLen)
		fmt.Printf("received message length: %d\n", length)

		if length == 0 {
			continue
		}

		message := make([]byte, length)
		_, err = io.ReadFull(peerConn.Conn, message)
		if err != nil {
			if !reconnect(fmt.Sprintf("Failed to read message: %s", err)) {
				return
			}
			continue
		}

		messageId := message[0]
		payload := message[1:]
		fmt.Printf("message ID: %d (payload size: %d bytes)\n", messageId, len(payload))

		switch messageId {
		case byte(Choke):
			if !reconnect("Got Choke") {
				return
			}

		case byte(Request):
			fmt.Println("peer requested a piece")
			pieceIdx := binary.BigEndian.Uint32(payload[0:4])
			if piecesCheck[int(pieceIdx)] {
				sendPiece(peerConn, torrent, int(pieceIdx), downloadPath)
			}

		case byte(Bitfield):
			fmt.Println("received bitfield")
			hasPiecesINeed := false
			numPieces := len(torrent.Info.Pieces)

			for i := 0; i < len(payload)*8 && i < numPieces; i++ {
				byteIndex := i / 8
				bitIndex := uint(i % 8)
				hasPiece := (payload[byteIndex] >> (7 - bitIndex)) & 1

				if hasPiece == 1 && !piecesCheck[i] {
					hasPiecesINeed = true
					break
				}
			}

			if hasPiecesINeed {
				sendInterested(peerConn)
				fmt.Println("sent Interested!")
			} else {
				sendNotInterested(peerConn)
				fmt.Println("sent Not Interested")
			}

		case byte(Unchoke):
			fmt.Println("got unchoke, requesting one piece")
			numPieces := len(torrent.Info.Pieces)
			for i := 0; i < numPieces; i++ {
				if !piecesCheck[i] {
					sendRequest(torrent, peerConn, i, downloadPath, receivedBlocks, pendingRequests)
					break
				}
			}

		case byte(Have):
			pieceIndex := int(binary.BigEndian.Uint32(payload[0:4]))
			fmt.Printf("peer has piece %d\n", pieceIndex)
			if !piecesCheck[pieceIndex] {
				sendRequest(torrent, peerConn, pieceIndex, downloadPath, receivedBlocks, pendingRequests)
			}

		case byte(Piece):
			pieceIndex := int(binary.BigEndian.Uint32(payload[0:4]))
			begin := int(binary.BigEndian.Uint32(payload[4:8]))
			data := payload[8:]

			fmt.Printf("received piece %d block at offset %d (%d bytes)\n", pieceIndex, begin, len(data))

			if piecesCheck[pieceIndex] {
				sendNotInterested(peerConn)
				break
			}

			if pendingRequests[pieceIndex] != nil {
				delete(pendingRequests[pieceIndex], begin)
			}

			if receivedBlocks[pieceIndex] == nil {
				receivedBlocks[pieceIndex] = make(map[int]bool)
			}
			receivedBlocks[pieceIndex][begin] = true

			success := WritePieceToFile(torrent, pieceIndex, data, downloadPath)
			
			if !success{
				fmt.Printf("error saving block: %s\n", err)
				break
			}
			fmt.Printf("saved block for piece %d at offset %d\n", pieceIndex, begin)

			pieceLength := int(torrent.Info.Piece_length)
			blockSize := 16384
			totalBlocks := (pieceLength + blockSize - 1) / blockSize

			if len(receivedBlocks[pieceIndex]) < totalBlocks {
				sendRequest(torrent, peerConn, pieceIndex, downloadPath, receivedBlocks, pendingRequests)
				break
			}

			fmt.Printf("piece %d is complete\n", pieceIndex)

			fullPiece := ReadPieceFromFiles(torrent, pieceIndex, downloadPath)
			if len(fullPiece) == 0 {
				fmt.Printf("failed to read piece %d from files\n", pieceIndex)
				delete(receivedBlocks, pieceIndex)
				delete(pendingRequests, pieceIndex)
				sendRequest(torrent, peerConn, pieceIndex, downloadPath, receivedBlocks, pendingRequests)
				break
			}

			h := sha1.New()
			h.Write(fullPiece)
			hash := h.Sum(nil)

			expectedHash := torrent.Info.Pieces[pieceIndex]

			if !bytes.Equal(hash, expectedHash[:]) {
				fmt.Printf("piece %d verification failed, re-requesting...\n", pieceIndex)
				delete(receivedBlocks, pieceIndex)
				delete(pendingRequests, pieceIndex)
				sendRequest(torrent, peerConn, pieceIndex, downloadPath, receivedBlocks, pendingRequests)
				break
			}

			fmt.Printf("piece %d verified!\n", pieceIndex)

			// err = os.WriteFile(fmt.Sprintf("%s/piece_%d", downloadPath, pieceIndex), fullPiece, 0644)
			
			piecesCheck[pieceIndex] = true
			sendHave(peerConn, pieceIndex)
			delete(receivedBlocks, pieceIndex)
			delete(pendingRequests, pieceIndex)
			numPieces := len(torrent.Info.Pieces)
			for i := 0; i < numPieces; i++ {
				if !piecesCheck[i] {
					fmt.Printf("requesting next piece: %d\n", i)
					sendRequest(torrent, peerConn, i, downloadPath, receivedBlocks, pendingRequests)
					break
				}
			}

			allDone := true
			for i := 0; i < numPieces; i++ {
				if !piecesCheck[i] {
					allDone = false
					break
				}
			}
			if allDone {
				fmt.Println("all pieces downloaded!")
				return
			}

		default:
			fmt.Printf("Unknown message ID: %d\n", messageId)
		}
	}
}

func sendRequest(torrent *core.TorrentMetaData, peerConn *PeerConn, pieceIdx int, downloadPath string, receivedBlocks map[int]map[int]bool,
	pendingRequests map[int]map[int]bool) {

	blockSize := 16384
	pieceLength := int(torrent.Info.Piece_length)


	const maxRequests = 20
	requested := 0

	for begin := 0; begin < pieceLength && requested < maxRequests; begin += blockSize {

		// blockFile := fmt.Sprintf("%s/piece_%d_block_%d", downloadPath, pieceIdx, begin)
		// if _, err := os.Stat(blockFile); err == nil {
		// 	continue
		// }

		if receivedBlocks[pieceIdx] != nil && receivedBlocks[pieceIdx][begin] {
            continue  
        }

		if pendingRequests[pieceIdx] != nil && pendingRequests[pieceIdx][begin] {
			continue
		}

		actualSize := blockSize
		if begin+blockSize > pieceLength {
			actualSize = pieceLength - begin
		}

		msg := make([]byte, 17)
		binary.BigEndian.PutUint32(msg[0:4], 13)
		msg[4] = byte(Request)
		binary.BigEndian.PutUint32(msg[5:9], uint32(pieceIdx))
		binary.BigEndian.PutUint32(msg[9:13], uint32(begin))
		binary.BigEndian.PutUint32(msg[13:17], uint32(actualSize))
		peerConn.Conn.Write(msg)
		fmt.Printf("requested piece %d block at offset %d\n", pieceIdx, begin)

		if pendingRequests[pieceIdx] == nil {
			pendingRequests[pieceIdx] = make(map[int]bool)
		}
		pendingRequests[pieceIdx][begin] = true
		requested++
	}
}

func sendInterested(peerConn *PeerConn) {
	msg := make([]byte, 5)
	binary.BigEndian.PutUint32(msg[0:4], 1)
	msg[4] = byte(Interested)
	peerConn.Conn.Write(msg)
}

func sendNotInterested(peerConn *PeerConn) {
	msg := make([]byte, 5)
	binary.BigEndian.PutUint32(msg[0:4], 1)
	msg[4] = byte(NotInterested)
	peerConn.Conn.Write(msg)
}

func sendPiece(peerConn *PeerConn, torrent *core.TorrentMetaData, pieceIndex int, downloadPath string) {
    fullPiece := ReadPieceFromFiles(torrent, pieceIndex, downloadPath)
    if len(fullPiece) == 0 {
        fmt.Printf("Piece %d not available to send\n", pieceIndex)
        return
    }

    blockSize := 16384

    for begin := 0; begin < len(fullPiece); begin += blockSize {
        end := begin + blockSize
        if end > len(fullPiece) {
            end = len(fullPiece)
        }
        block := fullPiece[begin:end]

        msg := make([]byte, 13+len(block))
        binary.BigEndian.PutUint32(msg[0:4], uint32(9+len(block)))
        msg[4] = byte(Piece)
        binary.BigEndian.PutUint32(msg[5:9], uint32(pieceIndex))
        binary.BigEndian.PutUint32(msg[9:13], uint32(begin))
        copy(msg[13:], block)
        peerConn.Conn.Write(msg)
    }
    fmt.Printf("Sent piece %d (%d bytes) to peer\n", pieceIndex, len(fullPiece))
}


func sendHave(peerConn *PeerConn, pieceIndex int) {
	msg := make([]byte, 9)
	binary.BigEndian.PutUint32(msg[0:4], 5)
	msg[4] = byte(Have)
	binary.BigEndian.PutUint32(msg[5:9], uint32(pieceIndex))
	peerConn.Conn.Write(msg)
	fmt.Printf("sent Have for piece %d\n", pieceIndex)
}