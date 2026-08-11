package main

import (

	"log"
	"os"
	"torrent/utils"
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


	
	metadata, err := utils.BuildMetaData(data, 0)
	utils.PrintMetaData(metadata)

}
