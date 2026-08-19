package utils

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"torrent/core"
)

// Integer: i<Value>e, 10->i10e
// String: <length>:<content> , peek->4:peek
// List: l<element1><element2>...<elementn>e, ["spam",4]->l4:spami4ee
// Dictionary: d<element1>...<elementn>e, {"foo":"bar","spam":"42"}->d3:foo3:bar4:spami42ee
// List and Dicts arent restricted to use a certain data type they can contain any

type BencodeKind int


const (
	KindString BencodeKind = iota
	KindInt 
	KindList
	KindDict
)

type BencodeNode struct {
	Kind BencodeKind
	Str string
	Int int64
	List []*BencodeNode
	Dict map[string]*BencodeNode
}

var numericRegex = regexp.MustCompile(`^[0-9]`)

func ParseValue(buffer []byte, i *int) (*BencodeNode, error){
	switch {
	case buffer[*i] == 'i':
		return ParseInteger(buffer, i)
	case numericRegex.MatchString(string(buffer[*i])):
		return ParseString(buffer, i)
	case buffer[*i] == 'l':
		return ParseList(buffer, i)
	case buffer[*i] == 'd': 
		return ParseDict(buffer, i)
	default:
		return nil, fmt.Errorf("unexpected data type, buffer hit: %q", buffer[*i])
	}
}


func ParseInteger(buffer []byte, i *int) (*BencodeNode, error){
	bencodeNode := BencodeNode{
		Kind: KindInt,
		Int: 0,
	}
	*i++ // pass 'i'
	start := *i
	for *i<len(buffer) && buffer[*i]!='e'{
		*i++
	}
	value, err := strconv.Atoi(string(buffer[start:*i]))
	*i++ //pass 'e'
	bencodeNode.Int += int64(value)
	return &bencodeNode, err
}

func ParseString(buffer []byte, i *int) (*BencodeNode, error){
	bencodeNode := BencodeNode{
		Kind: KindString,
	}
	
	start:=*i
	for *i<len(buffer) && numericRegex.MatchString(string(buffer[*i])){
		*i++
	}
	stringLen, err := strconv.Atoi(string(buffer[start:*i]))
	*i++ // pass ':'
	stringValue := string(buffer[*i:*i+stringLen])
	*i+=stringLen
	

	bencodeNode.Str = stringValue
	return &bencodeNode, err
}

func ParseList(buffer []byte, i *int) (*BencodeNode, error){
	*i++ //skip 'l'
	bencodeNode := BencodeNode{
		Kind: KindList,
	}
	for *i<len(buffer) && buffer[*i]!='e' {
		value, err := ParseValue(buffer, i)
		if err != nil {
			return nil, err
		}
		bencodeNode.List = append(bencodeNode.List, value)
	}
	if *i >= len(buffer) {
		return nil, fmt.Errorf("unexpected end while parsing list")
	}
	*i++ //skip 'e'
	return &bencodeNode, nil
}

func ParseDict(buffer []byte, i *int) (*BencodeNode, error){
	*i++ // skip d
	bencodeNode := BencodeNode{
		Kind: KindDict,
		Dict: make(map[string]*BencodeNode),
	}

	for *i<len(buffer) && buffer[*i]!='e'{
		key, err := ParseString(buffer, i)

		if err!=nil {
			return nil, err
		}

		value, err := ParseValue(buffer, i)
		if err != nil {
			return nil, err
		}
		bencodeNode.Dict[key.Str] = value
	}
	if *i >= len(buffer) {
		return nil, fmt.Errorf("unexpected end while parsing dictionary")
	}
	*i++ // skip 'e'
	return &bencodeNode, nil
}

//// just to verify the nested structure
// func PrintTorrent(node *BencodeNode, depth int ){

// 	if node == nil{
// 		return
// 	}

// 	indent := strings.Repeat(" ", depth)

// 	switch node.Kind{
// 	case KindDict:
// 		if depth > 0{
// 			fmt.Printf("%s{\n", indent)
// 		}
// 		for key, child := range node.Dict {
// 			fmt.Printf("%s %s: ",indent, key)
// 			PrintTorrent(child, depth+1)
// 		}
// 		if depth >0 {
// 			fmt.Printf("%s}\n", indent)
// 		}
	
// 	case KindList: 
// 		fmt.Printf("%s[\n", indent)
// 		for i, child := range node.List {
// 			fmt.Printf("%s [%d]", indent, i)
// 			PrintTorrent(child, depth+1)
// 		}
// 		fmt.Printf("%s]\n", indent)

// 	case KindString:
// 		fmt.Printf("%s\n", node.Str)
	
// 	case KindInt:
// 		fmt.Printf("%d\n", node.Int)
// 	}
// }

func BuildMetaData(buffer []byte,depth int) (*core.TorrentMetaData, error){
	root, err := ParseDict(buffer, &depth)
	if err!=nil{
		return nil, fmt.Errorf("%v\n",err)
	}
	metadata := core.TorrentMetaData{}
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
		infoData := &core.Info{}

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
			var filesList []core.FileItem
			
			for _, fileNode := range fileNode.List{
				if fileNode.Kind == KindDict{
					item := core.FileItem{}
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

func PrintMetaData(metadata *core.TorrentMetaData){
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