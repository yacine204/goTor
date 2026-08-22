package core

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

type TorrentMetaData struct {
	Announce string 
	Announce_list []string
	Info *Info
	RawInfoBytes []byte
}

type Info struct{
	Piece_length int64
	Pieces [][20]byte
	Name string 
	Length int64
	Files []FileItem

}

type FileItem struct{
	Length int64
	FilePath []string
	Offset int64
}



func InitTorrentFileOutput(torrent *Info, downloadPath string) {
	root := path.Join(downloadPath, torrent.Name)
	os.Mkdir(root, 0755)
	if len(torrent.Files) == 0{
		filePath := filepath.Join(torrent.Name,torrent.Name)
		file, err := os.Create(filePath)
		err = file.Truncate(torrent.Length)
		
		if err!=nil{
			fmt.Printf("err: %s\n", err)
		}
	}else{
		
		for _, file := range torrent.Files{
			fullPath := filepath.Join(append([]string{root}, file.FilePath...)...)
			parentDir := filepath.Dir(fullPath)
			if parentDir!=root && parentDir!= "."{
				if err:=os.MkdirAll(parentDir, 0755); err!=nil{
					fmt.Printf("err creating directory %s: %s\n", parentDir, err)
                    continue
				}
			}

			_file , err:= os.Create(fullPath)
			_file.Truncate(file.Length)
			if err!=nil{
					fmt.Printf("err: %s\n",err)
			}
			defer _file.Close()

		}
	}
}


func BuildMetaData(buffer []byte,depth int) (*TorrentMetaData, error){
	root, err := ParseDict(buffer, &depth)
	if err!=nil{
		return nil, fmt.Errorf("%v\n",err)
	}
	metadata := TorrentMetaData{}
	rawInfoBytes, err := extractRawInfoBytes(buffer)
	
	if err!=nil{
		return nil, fmt.Errorf("failed to extract info bytes: %v", err)
	}
	metadata.RawInfoBytes = rawInfoBytes

	if announceNode, ok := root.Dict["announce"]; ok {
		metadata.Announce = announceNode.Str
	}

	if announceListNode, ok := root.Dict["announce-list"]; ok {
		var list []string 

		for _, subList := range announceListNode.List{
			for _, item := range subList.List {
				list = append(list, item.Str)
			}
		}
		metadata.Announce_list = list
	}

	if len(metadata.Announce_list) == 0 && metadata.Announce != "" {
		metadata.Announce_list = []string{metadata.Announce}
	}

	
	if infoNode, ok := root.Dict["info"]; ok && infoNode.Kind == KindDict{
		infoData := &Info{}

		if pLength, ok := infoNode.Dict["piece length"]; ok{
			infoData.Piece_length = pLength.Int
		}

		if pieces, ok := infoNode.Dict["pieces"]; ok{
			rawBytes := []byte(pieces.Str)

			if len(rawBytes)%20==0{
				totalPieces := len(rawBytes)/20
				parsedPieces := make([][20]byte, totalPieces)

				for i:=0 ; i<totalPieces; i++{
					copy(parsedPieces[i][:], rawBytes[i*20:(i+1)*20])
				}
				infoData.Pieces = parsedPieces
			}
		}

		if name, ok := infoNode.Dict["name"]; ok {
			infoData.Name = name.Str
		}

		if lengthNode, ok := infoNode.Dict["length"]; ok {
			infoData.Length = lengthNode.Int
		}

		if fileNode, ok := infoNode.Dict["files"]; ok {
			var filesList []FileItem
			
			for _, fileNode := range fileNode.List{
				if fileNode.Kind == KindDict{
					item := FileItem{}
					if fLength, ok := fileNode.Dict["length"]; ok {
						item.Length = fLength.Int
					}

					if fPath, ok := fileNode.Dict["path"]; ok {
						for _, pathSegmet := range fPath.List{
							item.FilePath = append(item.FilePath, pathSegmet.Str)
						}
						
					}
					filesList = append(filesList, item)
				}
			}
			infoData.Files = filesList

			var offset int64
			
			for i := range infoData.Files{
				infoData.Files[i].Offset = offset
				offset+=infoData.Files[i].Length
			}
		}
		metadata.Info = infoData
	}

	return &metadata, nil
}

func extractRawInfoBytes(data []byte) ([]byte, error) {
	key := []byte("4:info")
	start := bytes.Index(data, key)
	if start == -1 {
		return nil, fmt.Errorf("info key not found")
	}
	start += len(key)

	if data[start] != 'd' {
		return nil, fmt.Errorf("info value is not a dictionary")
	}

	end, err := skipBencodeDict(data, start)
	if err != nil {
		return nil, err
	}

	return data[start:end], nil
}

func skipBencodeDict(data []byte, pos int) (int, error) {
	if pos >= len(data) || data[pos] != 'd' {
		return 0, fmt.Errorf("expected dict at pos %d", pos)
	}
	pos++
	for pos < len(data) && data[pos] != 'e' {
		var err error
		pos, err = skipBencodeAny(data, pos) // key 
		if err != nil {
			return 0, err
		}
		pos, err = skipBencodeAny(data, pos) // value
		if err != nil {
			return 0, err
		}
	}
	if pos >= len(data) {
		return 0, fmt.Errorf("unterminated dict")
	}
	return pos + 1, nil // include the 'e'
}

func skipBencodeAny(data []byte, pos int) (int, error) {
	if pos >= len(data) {
		return 0, fmt.Errorf("unexpected end of data")
	}
	switch {
	case data[pos] == 'i': // integer
		end := pos + 1
		for end < len(data) && data[end] != 'e' {
			end++
		}
		if end >= len(data) {
			return 0, fmt.Errorf("malformed integer")
		}
		return end + 1, nil

	case data[pos] == 'l': // list
		pos++
		for pos < len(data) && data[pos] != 'e' {
			var err error
			pos, err = skipBencodeAny(data, pos)
			if err != nil {
				return 0, err
			}
		}
		if pos >= len(data) {
			return 0, fmt.Errorf("unterminated list")
		}
		return pos + 1, nil

	case data[pos] == 'd': // dict
		return skipBencodeDict(data, pos)

	default: // string: N:...
		start := pos
		for pos < len(data) && data[pos] != ':' {
			pos++
		}
		if pos >= len(data) {
			return 0, fmt.Errorf("malformed string length")
		}
		length, err := strconv.Atoi(string(data[start:pos]))
		if err != nil {
			return 0, err
		}
		pos++ // skip ':'
		pos += length
		if pos > len(data) {
			return 0, fmt.Errorf("string exceeds data bounds")
		}
		return pos, nil
	}
}

func PrintMetaData(metadata *TorrentMetaData){
	fmt.Printf(
		"Announce: %s\nAnnounce List: %v\n",
		metadata.Announce,
		metadata.Announce_list,
	)
	depth := 0
	indent := strings.Repeat("   ", depth)

	fmt.Println("Info:")
	fmt.Printf("%sName: %s\n", indent+"  ", metadata.Info.Name)
	// fmt.Printf("%sPieces Length (Total): %q\n", indent+"  ", metadata.Info.Pieces) 
	fmt.Printf("%sLength: %d\n", indent+"  ", metadata.Info.Length)
	fmt.Printf("%sPiece Length: %d\n", indent+"  ", metadata.Info.Piece_length)


	depth++
	if len(metadata.Info.Files) > 0 {
		fmt.Printf("%sFiles:\n", indent + "   ")
		fileIndent := strings.Repeat("   ", depth+2) 
		for _, file := range metadata.Info.Files {
			fmt.Printf(
				"%s %s %d bytes\n",
				fileIndent, file.FilePath, file.Length,
			)
		}
	}

	
}