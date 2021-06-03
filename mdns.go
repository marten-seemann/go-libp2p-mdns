package mdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/grandcat/zeroconf"
)

const (
	mdnsInstance = "_p2p"
	mdnsService  = "_udp"
	mdnsDomain   = "local."
)

func extractIPs(addrs []ma.Multiaddr) ([]string, error) {
	var ip4, ip6 string
	for _, addr := range addrs {
		network, hostport, err := manet.DialArgs(addr)
		if err != nil {
			continue
		}
		host, _, err := net.SplitHostPort(hostport)
		if err != nil {
			continue
		}
		if ip4 == "" && (network == "udp4" || network == "tcp4") {
			ip4 = host
		} else if ip6 == "" && (network == "udp6" || network == "tcp6") {
			ip6 = host
		}
	}
	ips := make([]string, 0, 2)
	if ip4 != "" {
		ips = append(ips, ip4)
	}
	if ip6 != "" {
		ips = append(ips, ip6)
	}
	if len(ips) == 0 {
		return nil, errors.New("didn't find any IP addresses")
	}
	return ips, nil
}

func StartServer(peerID peer.ID, addrs []ma.Multiaddr) error {
	txts := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		txts = append(txts, "/dnsaddr="+addr.String())
	}
	// We don't really care about the IP addresses, but the spec (and various routers / firewalls) require us
	// to send A and AAAA records.
	ips, err := extractIPs(addrs)
	if err != nil {
		return err
	}

	server, err := zeroconf.RegisterProxy(
		mdnsInstance,
		mdnsService,
		mdnsDomain,
		4001,
		peerID.Pretty(), // TODO: deals with peer IDs longer than 63 characters
		ips,
		txts,
		nil,
	)
	if err != nil {
		return err
	}
	defer server.Shutdown()
	time.Sleep(10 * time.Second)
	return nil
}

func StartClient() error {
	resolver, err := zeroconf.NewResolver()
	if err != nil {
		return err
	}

	entryChan := make(chan *zeroconf.ServiceEntry, 10)
	go func() {
		for entry := range entryChan {
			fmt.Printf("received entry for %s: %#v\nIPv4:", entry.ServiceInstanceName(), entry)
			for _, ip := range entry.AddrIPv4 {
				fmt.Printf("\t%s\n", ip.String())
			}
			fmt.Print("IPv6:")
			for _, ip := range entry.AddrIPv6 {
				fmt.Printf("\t%s\n", ip.String())
			}
		}
	}()
	if err := resolver.Lookup(context.Background(), mdnsInstance, mdnsService, mdnsDomain, entryChan); err != nil {
		return err
	}
	time.Sleep(10 * time.Second)
	return nil
}
