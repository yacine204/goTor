package utils

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type PeerConn struct {
	Conn net.Conn
	Peer PeerNode
}

func ConnectToPeer(peers []PeerNode) (PeerConn, error){ 
	var wg sync.WaitGroup
	var mutex sync.Mutex

	var peerConn PeerConn
	var wonRace bool

	for _, peer := range peers {
		wg.Add(1)

		go func(p PeerNode){
			defer wg.Done()

			conn, err := net.DialTimeout("tcp", net.JoinHostPort(p.Ip, p.Port), 5*time.Second)
			if err!=nil{
				fmt.Printf("error: %s\n",err)
				return 
			}

			mutex.Lock()
			defer mutex.Unlock()

			if wonRace{
				// fmt.Printf("%s lost the race!\n", p.Ip)
				conn.Close()
				return
			}

			wonRace = true
			peerConn = PeerConn{
				Peer: p,
				Conn: conn,
			}
			// fmt.Printf("%s won the race!\n",p.Ip)
		}(peer)
		
	}

	wg.Wait()

	if !wonRace{
		return PeerConn{}, fmt.Errorf("no peers could be connected!\n")
	}
	return peerConn, nil
}