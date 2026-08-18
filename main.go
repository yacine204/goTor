package main

import (
	"fmt"
	"log"
	"os"
	"torrent/utils"
)

func main()  {
	file, err := os.Open("The First Berserker - Khazan [FitGirl Repack].torrent")

	if err!= nil {
		log.Fatal(err)
	}

	defer file.Close()

	fileInfo, err := file.Stat()
	if err!= nil {
		log.Fatal(err)
	}
	fileSize := fileInfo.Size()
	data := make([]byte, fileSize)
	_, err = file.Read(data)


	pos := 0
	metadata, err := utils.BuildMetaData(data, pos)


	// for i := range metadata.Announce_list{
	// 	peer, err := utils.ConnectToTracker(metadata, i)
	// 	if err!=nil{
	// 		fmt.Printf("error %s\n", err)
	// 		continue
	// 	}
	// 	fmt.Printf("Peers from announce %d :\n",i)
	// 	for j := range peer{
	// 		fmt.Printf("peer %d: %s:%s", j,peer[j].Ip, peer[j].Port)
	// 	}
		
	// }


	peers := utils.ConnectTrackersAsync(metadata)
	
	for i, peer := range peers {
		fmt.Printf("peer %d: %s %s\n", i, peer.Ip, peer.Port)
	}

	peerConn, err := utils.ConnectToPeer(peers)
	if err!=nil{
		fmt.Printf("err: %s\n", err)
	}

	fmt.Printf("%s %s\n", peerConn.Peer.Ip, peerConn.Peer.Port)
}
