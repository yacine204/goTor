package main

import (
	"log"
	"os"
	"torrent/utils/bencode"

)

func main()  {
	file, err := os.Open("ELDEN RING [FitGirl Repack].torrent")

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
	value, err := bencode.ParseValue(data, &pos)
	if err!= nil {
		log.Fatalf("Error: %v", err)
	}
	bencode.PrintTorrent(value, 0)

}
