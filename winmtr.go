// +build windows

package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// WinMTRHop содержит информацию об одном хопе
type WinMTRHop struct {
	Hop      int
	Address  string
	RTT      time.Duration
	Success  bool
}

// winMTR выполняет ICMP-трассировку до host с maxHops
func winMTR(host string, maxHops int, timeout time.Duration) ([]WinMTRHop, error) {
	var hops []WinMTRHop
	ipAddr, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return nil, fmt.Errorf("не удалось разрешить адрес: %v", err)
	}

	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть ICMP сокет: %v", err)
	}
	defer conn.Close()

	for ttl := 1; ttl <= maxHops; ttl++ {
		start := time.Now()
		wmsg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   os.Getpid() & 0xffff,
				Seq:  ttl,
				Data: []byte("PINGSTATSMTR"),
			},
		}
		wb, err := wmsg.Marshal(nil)
		if err != nil {
			return nil, fmt.Errorf("marshal icmp: %v", err)
		}

		pconn := conn.IPv4PacketConn()
		if err := pconn.SetTTL(ttl); err != nil {
			return nil, fmt.Errorf("set ttl: %v", err)
		}
		if _, err := conn.WriteTo(wb, &net.IPAddr{IP: ipAddr.IP}); err != nil {
			hops = append(hops, WinMTRHop{Hop: ttl, Address: "*", RTT: 0, Success: false})
			continue
		}

		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		rb := make([]byte, 1500)
		n, peer, err := conn.ReadFrom(rb)
		if err != nil {
			hops = append(hops, WinMTRHop{Hop: ttl, Address: "*", RTT: 0, Success: false})
			continue
		}
		rtt := time.Since(start)
		msg, err := icmp.ParseMessage(1, rb[:n])
		if err != nil {
			hops = append(hops, WinMTRHop{Hop: ttl, Address: "?", RTT: rtt, Success: false})
			continue
		}
		addr := peer.String()
		if msg.Type == ipv4.ICMPTypeTimeExceeded {
			hops = append(hops, WinMTRHop{Hop: ttl, Address: addr, RTT: rtt, Success: true})
		} else if msg.Type == ipv4.ICMPTypeEchoReply {
			hops = append(hops, WinMTRHop{Hop: ttl, Address: addr, RTT: rtt, Success: true})
			break // достигли цели
		} else {
			hops = append(hops, WinMTRHop{Hop: ttl, Address: addr, RTT: rtt, Success: false})
		}
	}
	return hops, nil
}

// Форматированный вывод для CLI/GUI
func FormatWinMTRResult(hops []WinMTRHop) string {
	result := "Hop\tAddress\t\tRTT (ms)\tSuccess\n"
	for _, h := range hops {
		result += fmt.Sprintf("%d\t%s\t%.2f\t%v\n", h.Hop, h.Address, h.RTT.Seconds()*1000, h.Success)
	}
	return result
} 