package main

import (
	"fmt"
	"log"
	"os"
	"torrent/utils"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("USAGE: torrent <torrent file> <download path>\n")
		return
	}

	torrentFile := os.Args[1]
	downloadPath := os.Args[2]

	if torrentFile == "" {
		fmt.Printf("select a torrent file\n")
		return
	}

	if downloadPath == "" {
		fmt.Printf("select a download path\n")
		return
	}

	file, err := os.Open(torrentFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Fatal(err)
	}
	fileSize := fileInfo.Size()
	data := make([]byte, fileSize)
	_, err = file.Read(data)
	if err != nil {
		log.Fatal(err)
	}

	pos := 0
	metadata, err := utils.BuildMetaData(data, pos)
	if err != nil {
		log.Fatalf("failed to parse torrent: %s", err)
	}

	peers := utils.ConnectTrackersAsync(metadata)
	fmt.Printf("Found %d peers\n", len(peers))

	for i, peer := range peers {
		fmt.Printf("peer %d: %s %s\n", i, peer.Ip, peer.Port)
	}

	if len(peers) == 0 {
		fmt.Println("No peers found, giving up")
		return
	}

	peerConn, err := utils.ConnectToPeer(peers)
	if err != nil {
		fmt.Printf("Failed to connect to peer: %s\n", err)
		return
	}

	fmt.Printf("Connected to peer: %s:%s\n", peerConn.Peer.Ip, peerConn.Peer.Port)

	_, state, err := utils.BitTorrentHandShake(&peerConn, metadata)
	if err != nil {
		fmt.Printf("Handshake error: %s\n", err)
		peerConn.Conn.Close()
		return
	}

	if !state {
		fmt.Printf("Handshake failed!\n")
		peerConn.Conn.Close()
		return
	}

	fmt.Println("Handshake successful!")

	numPieces := len(metadata.Info.Pieces)
	piecesCheck := make(map[int]bool, numPieces)
	for i := 0; i < numPieces; i++ {
		piecesCheck[i] = false
	}

	utils.BitLoop(&peerConn, metadata, piecesCheck, downloadPath)
}