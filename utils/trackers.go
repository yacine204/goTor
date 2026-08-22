package utils

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
	"torrent/core"
	
)


type AnnConnType string

const (
	HTTP AnnConnType = "http"
	HTTPS AnnConnType = "https"
	UDP AnnConnType = "udp"
)

type AnnConn struct{
	List map[AnnConnType][]net.Conn
}

type AnounceFragment struct {
	scheme AnnConnType
	host string
	port string
	fullAddr string
}

type PeerNode struct {
	Ip string
	Port string
}

func StripConn(announce *string) (*AnounceFragment , error){
	
	url, err := url.Parse(*announce)
	if err!=nil{
		return nil, err 
	}
	port := url.Port()

	if port == "" {
		switch url.Scheme{
		case string(HTTP):
			port = "80"
		case string(HTTPS):
			port = "443"
		case string(UDP):
			port = "6969"
		}
	}

	announceFragment := &AnounceFragment{
		scheme: AnnConnType(url.Scheme),
		host: url.Hostname(),
		port: port,
		fullAddr: net.JoinHostPort(url.Hostname(), port),
	}
	return announceFragment, nil

}	


func GetUDPPeers(torrent *core.TorrentMetaData , announceFragments *AnounceFragment, conn net.Conn) ([]PeerNode, error){
	buf := make([]byte, 16)
	transactionID := rand.Uint32()
	var peers []PeerNode

	binary.BigEndian.PutUint64(buf[0:8], 0x41727101980)
	binary.BigEndian.PutUint32(buf[8:12], 0)
	binary.BigEndian.PutUint32(buf[12:16], transactionID)

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err := conn.Write(buf)
	if err!=nil{
		fmt.Printf("error: %s\n", err)
		return nil, err
	}
	bufRes := make([]byte, 16)

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Read(bufRes)
	if err!=nil{
		fmt.Printf("error: %s\n", err)
		return nil, err
	}
			
	connectionIDRes := bufRes[0:8]
	// actionRes := bufRes[8:12]
	transactionIDRes := bufRes[12:16]

	if binary.BigEndian.Uint32(transactionIDRes) == transactionID{
		annReq := make([]byte, 98)
		copy(annReq[0:8], connectionIDRes)
		binary.BigEndian.PutUint32(annReq[8:12], 1)
		binary.BigEndian.PutUint32(annReq[12:16], transactionID)


		h := sha1.New()
		h.Write(torrent.RawInfoBytes)
		info_hash := h.Sum(nil)
		fmt.Printf("hash: %d", info_hash[:])
		copy(annReq[16:36], info_hash)

		// todo: check client_id dynamic value for qbit and utor
		peerID := "-TR0001-" + fmt.Sprintf("%012d", rand.Int64()%1000000000000)
		copy(annReq[36:56], []byte(peerID))

		// downloaded
		binary.BigEndian.PutUint64(annReq[56:64], 0)

		// left 
		binary.BigEndian.PutUint64(annReq[64:72], uint64(torrent.Info.Length))

		// uploaded 
		binary.BigEndian.PutUint64(annReq[72:80], 0)

		// event (0=none, 1=completed, 2=started, 3=stopped)
		binary.BigEndian.PutUint32(annReq[80:84], 2)

		// ip (0 auto)
		binary.BigEndian.PutUint32(annReq[84:88], 0)

		// key (random key)
		binary.BigEndian.PutUint32(annReq[88:92], rand.Uint32())

		// num_want (Peers wanted (-1 = default))
		binary.BigEndian.PutUint32(annReq[92:96], 0xFFFFFFFF)

		// port 
		portUint64, err := strconv.ParseUint(announceFragments.port, 10, 16)
		if err!=nil{
			fmt.Printf("error: %s\n", err)
			return nil, err
		}
		binary.BigEndian.PutUint16(annReq[96:98], uint16(portUint64))

		conn.SetDeadline(time.Now().Add(5 * time.Second))
		_, err = conn.Write(annReq)

		if err!=nil{
			return nil, err
		}
				
		bufAnnResHeader := make([]byte, 20)

		conn.SetDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(bufAnnResHeader)

		if err!=nil{
			return nil, err
		}
		if n < 20 {
			return nil, fmt.Errorf("error: response too short: %d bytes\n", n)
		}

		var allPeersData []byte
    	readBuf := make([]byte, 4096)

		for {
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			n, err := conn.Read(readBuf)
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			if n == 0 {
				break
			}
			allPeersData = append(allPeersData, readBuf[:n]...)
			if n < len(readBuf) {
				break
			}
		}
		
		for i := 0; i+6 <= len(allPeersData); i += 6 {
			ipBytes := allPeersData[i : i+4]
			portBytes := allPeersData[i+4 : i+6]
			ip := net.IP(ipBytes).String()
			port := binary.BigEndian.Uint16(portBytes)
			peers = append(peers, PeerNode{
				Ip:   ip,
				Port: strconv.Itoa(int(port)),
			})
   	 }

		
	}
	return peers, nil
}

func GetHTTPsPeers(torrent *core.TorrentMetaData, announce string) ([]PeerNode, error){
	h := sha1.New()
	h.Write(torrent.RawInfoBytes)
	infoHash := h.Sum(nil)
	peerID := "-TR0001-" + fmt.Sprintf("%012d", rand.Int64()%1000000000000)
	query := url.Values{}
	query.Set("info_hash", string(infoHash))
	query.Set("peer_id", peerID)
	query.Set("port", "6881")
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	query.Set("left", strconv.FormatInt(torrent.Info.Length, 10))
	query.Set("event", "started")
	query.Set("compact", "1")
	announceURL := fmt.Sprintf("%s?%s", announce, query.Encode())
	res, err := http.Get(announceURL)
	if err!=nil{
		return nil, err
	}
	defer res.Body.Close()
	bodyRes, err := io.ReadAll(res.Body)
	
	if err!=nil{
		return nil, err
	}
	pos := 0
	bencodeNode, err := core.ParseValue(bodyRes, &pos)
	if err!=nil{
		return nil, err
	}
	if bencodeNode.Kind != core.KindDict {
		return nil, fmt.Errorf("error: response is not a dictionary\n")
	}
	if failureNode, ok := bencodeNode.Dict["failure reason"]; ok {
		fmt.Printf("tracker failure: %s", failureNode.Str)
	}
	
	peersNode, ok := bencodeNode.Dict["peers"]
	if !ok {
    	return nil , fmt.Errorf("error: no peers in response\n")
	}
	var peers  []PeerNode
	if peersNode.Kind == core.KindString{
		peersData := []byte(peersNode.Str)
		for i := 0; i+6 <= len(peersData); i += 6 {
			ip := net.IP(peersData[i:i+4]).String()
			port := binary.BigEndian.Uint16(peersData[i+4:i+6])
			peers = append(peers, PeerNode{Ip: ip, Port: strconv.Itoa(int(port))})
		}
	}else if peersNode.Kind == core.KindList{
		for _, peerNode := range peersNode.List{
			if peerNode.Kind == core.KindDict{
				var ip, port string
				if ipNode, ok := peerNode.Dict["ip"]; ok && ipNode.Kind == core.KindString{
					ip = ipNode.Str
				}
				if portNode, ok := peerNode.Dict["port"]; ok && portNode.Kind == core.KindInt {
					port = strconv.Itoa(int(portNode.Int))
				}
				if ip != "" && port != "" {
					peers = append(peers, PeerNode{Ip: ip, Port: port})
				}
			}
		}
	}
	return peers, nil
}

func ConnectToTrackers(torrent *core.TorrentMetaData) (*[]PeerNode, error){
	// opens a tcp/udp connection depending on the tracker scheme
	// dials all the trackers of the torrent 
	// returns all peers (ip, port)
	var allPeers []PeerNode
	for  _, announce := range torrent.Announce_list{
		
		announceFragments, err := StripConn(&announce)
		if err!=nil{
			fmt.Printf("error: %s\n", err)
			return nil, err
		}
		
		networkType := string(announceFragments.scheme)
		if announceFragments.scheme == HTTP || announceFragments.scheme == HTTPS {
			networkType = "tcp"
		}else{
			networkType = "udp"
		}
		conn, err := net.DialTimeout(networkType, announceFragments.fullAddr, 5*time.Second)
		if err!=nil{
			fmt.Printf("error: %s\n", err)
			continue
		}

		switch announceFragments.scheme{
		case UDP:
			peersUDP, err := GetUDPPeers(torrent, announceFragments, conn)
			if err!=nil {
				fmt.Printf("error: %s\n", err)
			}
			allPeers = append(allPeers, peersUDP...)
		
		case HTTP, HTTPS:

			peersHTTPs, err := GetHTTPsPeers(torrent, announce)
			if err!=nil{
				fmt.Printf("error: %s\n", err)
			}

			allPeers = append(allPeers, peersHTTPs...)
		}
			

	
	}
	return &allPeers, nil
}


func ConnectToTracker(torrent *core.TorrentMetaData, trackerIdx int) ([]PeerNode, error){
	var allPeers []PeerNode
	announce := torrent.Announce_list[trackerIdx]
	announceFragments, err := StripConn(&announce)
	if err!=nil{
		return nil, err
	}

	networkType := string(announceFragments.scheme)
	if announceFragments.scheme == HTTP || announceFragments.scheme == HTTPS {
		networkType = "tcp"
	}else{
		networkType = "udp"
	}

	conn, err := net.DialTimeout(networkType, announceFragments.fullAddr, 5*time.Second)

	if err!=nil{
		fmt.Printf("error: %s\n", err)
		return nil, err
	}
	defer conn.Close()

	switch announceFragments.scheme{
	case UDP:
		peersUDP, err := GetUDPPeers(torrent, announceFragments, conn)
		if err!=nil {
			fmt.Printf("error: %s\n", err)
		}
		allPeers = append(allPeers, peersUDP...)
	
	case HTTP, HTTPS:
		peersHTTPs, err := GetHTTPsPeers(torrent, announce)
		if err!=nil{
			fmt.Printf("error: %s\n", err)
		}
		allPeers = append(allPeers, peersHTTPs...)
	}
	

	return allPeers, nil
}


func ConnectTrackersAsync(torrent *core.TorrentMetaData) ([]PeerNode){
	// conccurently connect to all trackers returning a list of PeerNode {Ip, Port}
	var wg sync.WaitGroup
	var mutex sync.Mutex
	var allPeers []PeerNode


	for i := range torrent.Announce_list{
		wg.Add(1)

		go func(idx int){
			defer wg.Done()

			peers, err := ConnectToTracker(torrent, idx)
			if err!=nil{
				fmt.Printf("error: %s\n", err)
				return
			}

			mutex.Lock()
			allPeers = append(allPeers, peers...)
			mutex.Unlock()

			fmt.Printf("Announce %d got %d peers\n", idx, len(peers))
		}(i)
		
	}
	wg.Wait()
	allPeers = _eliminateDuplicatePeers(allPeers)
	return allPeers
}

func _eliminateDuplicatePeers(peers []PeerNode) ([]PeerNode){
	seen := make(map[string]bool, len(peers))
	var uniquePeers []PeerNode

	for _, peer := range peers{
		// deduplicate by ip (trackers overloads ports to distribute malware)
		// todo : check for suspicious peers and exclude them
		if !seen[peer.Ip]{
			seen[peer.Ip] = true
			uniquePeers = append(uniquePeers, peer)
		}	
	}
	return uniquePeers
}