package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
	"torrent/core"
	"torrent/utils"
	"io"
)

func main() {
	if len(os.Args) < 3{
		fmt.Printf("USAGE: torrent <torrent file> <download path>\n")
		return
	}

	torrentFile := os.Args[1]
	downloadPath := os.Args[2]

	_, err := os.Stat(downloadPath)

	if torrentFile == "" {
		fmt.Printf("torrent file cannot be empty\n")
		return
	}

	if downloadPath == "" {
		fmt.Printf("download path cannot be empty\n")
		return
	}

	if err!=nil{
		if os.IsNotExist(err){
			err := os.MkdirAll(downloadPath, 0755)
			if err!=nil{
				fmt.Printf("err: %s\n",err)
				return	
			}
		}else{
			fmt.Printf("err: %s\n",err)
			return
		}
	}

	_, err = os.Stat(torrentFile)

	if err!=nil{
		if os.IsNotExist(err){
			fmt.Printf("%s doesnt exist!\n", torrentFile)
			return
		}else{
			fmt.Printf("err: %s\n",err)
			return
		}
	}

	logDir := filepath.Join(downloadPath, "logs")
	err = os.MkdirAll(logDir, 0755)
	if err != nil {
		fmt.Printf("err: %s\n", err)
		return
	}


	logPath := filepath.Join(logDir, fmt.Sprintf("download_%d.log", time.Now().Unix()))
	logFile, err := os.Create(logPath)
	if err != nil {
		fmt.Printf("err: %s\n", err)
		return
	}
	defer logFile.Close()

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	go func(){
		io.Copy(io.MultiWriter(origStdout, logFile), r)
	}()
	
	data, err := os.ReadFile(torrentFile)
	if err!=nil{
		log.Fatalf("error: %s", err)
	}
	pos := 0
	metadata, err := core.BuildMetaData(data, pos)
	if err != nil {
		log.Fatalf("failed to parse torrent: %s", err)
	}

	core.InitTorrentFileOutput(metadata.Info, downloadPath)
	peerConn, err := utils.ConnectAndHandshake(metadata)
	if err != nil {
		fmt.Printf("Failed to connect: %s\n", err)
		return
	}

	fmt.Printf("Connected to peer: %s:%s\n", peerConn.Peer.Ip, peerConn.Peer.Port)
	fmt.Println("Handshake successful!")

	numPieces := len(metadata.Info.Pieces)
	piecesCheck := make(map[int]bool, numPieces)
	for i := 0; i < numPieces; i++ {
		piecesCheck[i] = false
	}

	utils.BitLoop(peerConn, metadata, piecesCheck, downloadPath)
}