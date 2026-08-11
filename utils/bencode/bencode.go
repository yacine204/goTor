package bencode

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
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

// just to verify the nested structure
func PrintTorrent(node *BencodeNode, depth int ){

	if node == nil{
		return
	}

	indent := strings.Repeat(" ", depth)

	switch node.Kind{
	case KindDict:
		if depth > 0{
			fmt.Printf("%s{\n", indent)
		}
		for key, child := range node.Dict {
			fmt.Printf("%s %s: ",indent, key)
			PrintTorrent(child, depth+1)
		}
		if depth >0 {
			fmt.Printf("%s}\n", indent)
		}
	
	case KindList: 
		fmt.Printf("%s[\n", indent)
		for i, child := range node.List {
			fmt.Printf("%s [%d]", indent, i)
			PrintTorrent(child, depth+1)
		}
		fmt.Printf("%s]\n", indent)

	case KindString:
		fmt.Printf("%s\n", node.Str)
	
	case KindInt:
		fmt.Printf("%d\n", node.Int)
	}
}