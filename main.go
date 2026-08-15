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

	fmt.Printf("Announce: %s\n", metadata.Announce)
	for i, announce := range metadata.Announce_list{
		fmt.Printf("[Announce %d]: %s\n", i, announce)
	}
	peers, err := utils.ConnectToTrackers(metadata)
	if err!=nil {
		log.Fatal(err)
	}
	
	for i, peer := range *peers {
		fmt.Printf("[%d] IP: %s, Port: %s\n", i, peer.Ip, peer.Port)
	}	

}
