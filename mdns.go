package mdns

import (
	"context"
	"errors"
	"fmt"
	"net"

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

type MDNSService struct {
	id    peer.ID
	addrs []ma.Multiaddr

	// This ctx is passed to the resolver.
	// It is closed when Close() is called.
	ctx       context.Context
	ctxCancel context.CancelFunc

	server *zeroconf.Server
}

func NewMDNSService(id peer.ID, addrs []ma.Multiaddr) *MDNSService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &MDNSService{
		ctx:       ctx,
		ctxCancel: cancel,
		id:        id,
		addrs:     addrs,
	}
	s.startServer()
	s.startResolver()
	return s
}

func (s *MDNSService) Close() error {
	s.ctxCancel()
	if s.server != nil {
		s.server.Shutdown()
	}
	return nil
}

// We don't really care about the IP addresses, but the spec (and various routers / firewalls) require us
// to send A and AAAA records.
func (s *MDNSService) getIPs() ([]string, error) {
	var ip4, ip6 string
	for _, addr := range s.addrs {
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

func (s *MDNSService) startServer() error {
	txts := make([]string, 0, len(s.addrs))
	for _, addr := range s.addrs {
		txts = append(txts, "/dnsaddr="+addr.String())
	}

	ips, err := s.getIPs()
	if err != nil {
		return err
	}

	server, err := zeroconf.RegisterProxy(
		mdnsInstance,
		mdnsService,
		mdnsDomain,
		4001,
		s.id.Pretty(), // TODO: deals with peer IDs longer than 63 characters
		ips,
		txts,
		nil,
	)
	if err != nil {
		return err
	}
	s.server = server
	return nil
}

func (s *MDNSService) startResolver() error {
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
	return resolver.Lookup(s.ctx, mdnsInstance, mdnsService, mdnsDomain, entryChan)
}
