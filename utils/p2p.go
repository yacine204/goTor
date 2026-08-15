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


func ConnectToPeers(torrent *core.TorrentMetaData) (*[]PeerNode, error){
	// udp doesnt work...
	// todo : look into Distributed Hash Table protocol
	// todo : add goroutines to trackers
	annConnList := &AnnConn{
		List : make(map[AnnConnType][]net.Conn),

	}
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
		annConnList.List[announceFragments.scheme] = append(annConnList.List[announceFragments.scheme],conn)	
		
		switch announceFragments.scheme{
		case UDP:
			buf := make([]byte, 16)
			transactionID := rand.Uint32()

			binary.BigEndian.PutUint64(buf[0:8], 0x41727101980)
			binary.BigEndian.PutUint32(buf[8:12], 0)
			binary.BigEndian.PutUint32(buf[12:16], transactionID)

			conn.SetDeadline(time.Now().Add(5 * time.Second))
			_, err := conn.Write(buf)
			if err!=nil{
				fmt.Printf("error: %s\n", err)
				continue
			}
			bufRes := make([]byte, 16)
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			_, err = conn.Read(bufRes)
			if err!=nil{
				fmt.Printf("error: %s\n", err)
				continue
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
				}
				binary.BigEndian.PutUint16(annReq[96:98], uint16(portUint64))

				conn.SetDeadline(time.Now().Add(5 * time.Second))
				_, err = conn.Write(annReq)

				if err!=nil{
					fmt.Printf("error: %s\n", err)
					continue
				}
				
				bufAnnResHeader := make([]byte, 20)

				conn.SetDeadline(time.Now().Add(5 * time.Second))
				n, err := conn.Read(bufAnnResHeader)

				if err!=nil{
					fmt.Printf("error: %s\n", err)
					continue
				}
				if n < 20 {
					fmt.Printf("error: response too short: %d bytes\n", n)
					continue
				}

				// action := binary.BigEndian.Uint32(bufAnnResHeader[0:4])
				// transactionIDRes := binary.BigEndian.Uint32(bufAnnResHeader[4:8])
				// interval := binary.BigEndian.Uint32(bufAnnResHeader[8:12])
				// leechers := binary.BigEndian.Uint32(bufAnnResHeader[12:16])
				// seeders := binary.BigEndian.Uint32(bufAnnResHeader[16:20])
				fmt.Printf("Announce response header: action=%d, transaction=%d\n", 
						binary.BigEndian.Uint32(bufAnnResHeader[0:4]),
						binary.BigEndian.Uint32(bufAnnResHeader[4:8]))
				bufAnnResPeersList := make([]byte, 1024)
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				n2, err := conn.Read(bufAnnResPeersList)
				if err != nil {
					fmt.Printf("error: %s\n", err)
					continue
				}
				var peerMap []PeerNode
				for i:=0 ; i+6 <= n2; i+=6  {
					ipBytes := bufAnnResPeersList[i: i+4]
					portBytes := bufAnnResPeersList[i+4: i+6]
					
					ip := net.IP(ipBytes).String()
    				port := binary.BigEndian.Uint16(portBytes)

					peerMap = append(peerMap, PeerNode{
						Ip: ip,
						Port: strconv.Itoa(int(port)),
					})
				}
				allPeers = append(allPeers, peerMap...)
			}
		case HTTP, HTTPS:
			h := sha1.New()
			h.Write(torrent.RawInfoBytes)
			info_hash := h.Sum(nil)
			peerID := "-TR0001-" + fmt.Sprintf("%012d", rand.Int64()%1000000000000)

			query := url.Values{}
			query.Set("info_hash", string(info_hash))
			query.Set("peer_id", peerID)
			query.Set("uploaded", "0")
			query.Set("downloaded", "0")
			query.Set("left", fmt.Sprintf("%d", torrent.Info.Length))
			query.Set("port", announceFragments.port)
			query.Set("event", "started")
			query.Set("compact", "1")

			announceURL := fmt.Sprintf("%s?%s", announce, query.Encode())

			resp, err := http.Get(announceURL)

			if err!=nil{
				fmt.Printf("error: %s\n", err)
        		continue
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			
			if err != nil {
				fmt.Printf("error: %s\n", err)
				continue
			}

			pos:=0
			bencodeNode, err := ParseValue(body, &pos)

			if err!=nil{
				fmt.Printf("error parsing bencode: %s\n", err)
        		continue
			}

			if bencodeNode.Kind != KindDict {
				return nil, fmt.Errorf("response is not a dictionary")
			}

			if failureNode, ok := bencodeNode.Dict["failure reason"]; ok {
				return nil, fmt.Errorf("tracker failure: %s", failureNode.Str)
			}

			peersNode, ok := bencodeNode.Dict["peers"]
			if !ok {
				return nil, fmt.Errorf("no peers in response")
			}

			var peers []PeerNode

			if peersNode.Kind == KindString {
			
				peersData := []byte(peersNode.Str)
				for i := 0; i+6 <= len(peersData); i += 6 {
					ip := net.IP(peersData[i : i+4]).String()
					port := binary.BigEndian.Uint16(peersData[i+4 : i+6])
					peers = append(peers, PeerNode{Ip: ip, Port: strconv.Itoa(int(port))})
				}
			} else if peersNode.Kind == KindList {
				
				for _, peerNode := range peersNode.List {
					if peerNode.Kind == KindDict {
						var ip, port string
						if ipNode, ok := peerNode.Dict["ip"]; ok && ipNode.Kind == KindString {
							ip = ipNode.Str
						}
						if portNode, ok := peerNode.Dict["port"]; ok && portNode.Kind == KindInt {
							port = strconv.Itoa(int(portNode.Int))
						}
						if ip != "" && port != "" {
							peers = append(peers, PeerNode{Ip: ip, Port: port})
						}
					}
				}
				
			}
			allPeers = append(allPeers, peers...)
				
			}
		}
			

	return &allPeers, nil

}