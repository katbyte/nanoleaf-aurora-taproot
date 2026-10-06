// Package discover finds controllers on the local network: by listening for
// the way they announce themselves (mDNS, the service _nanoleafapi._tcp), and
// by knocking on every address of a subnet for when announcements do not
// carry that far.
package discover

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Service is the mDNS service type controllers announce.
const Service = "_nanoleafapi._tcp.local."

// DefaultPort is the API's port, used where an announcement does not say.
const DefaultPort = 16021

// maxScan is the most addresses Scan will knock on: a /20.
const maxScan = 4096

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// Found is a controller that answered.
type Found struct {
	// Name is what it calls itself: "Light Panels 53:A6:3C". Empty for one
	// found by Scan, which cannot ask.
	Name string `json:"name"`
	// Host is its mDNS host name, "Light-Panels-53-A6-3C.local".
	Host string `json:"host,omitempty"`
	// Addrs is the addresses it gave, IPv4 first.
	Addrs []netip.Addr `json:"addrs"`
	Port  int          `json:"port"`
	// Model, Firmware and ID are from its announcement: md, srcvers and id.
	// The ID changes when the controller is reset, and its tokens go with it.
	Model    string `json:"model,omitempty"`
	Firmware string `json:"firmware,omitempty"`
	ID       string `json:"id,omitempty"`
}

// Address is where to reach the controller: its first address, or its host
// name when it gave none, with the port when that is not the usual one.
func (f Found) Address() string {
	host := strings.TrimSuffix(f.Host, ".")
	if len(f.Addrs) > 0 {
		host = f.Addrs[0].String()
	}
	if f.Port != 0 && f.Port != DefaultPort {
		return net.JoinHostPort(host, strconv.Itoa(f.Port))
	}

	return host
}

// Browse listens for controllers for as long as wait and returns the ones
// that answered, by name. It asks twice in that time, since a question or an
// answer is easily lost.
//
// It asks in two ways at once. One is the usual: from the mDNS port, with the
// answers multicast, which is what a router relaying mDNS between networks
// passes on. The other is from a port of its own with the answers sent
// straight back, which works when something else on this machine holds the
// mDNS port to itself. On a Mac the system's own discovery service is asked
// as well, since a firewall there often keeps the answers to both from
// arriving.
func Browse(ctx context.Context, wait time.Duration) ([]Found, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	// the system's own discovery service is asked alongside, where there is one worth asking
	var system []Found
	var sys sync.WaitGroup
	sys.Go(func() { system = systemBrowse(ctx) })

	seen := newRecords()
	conns := listen()
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()

	var wg sync.WaitGroup
	for _, c := range conns {
		wg.Go(func() { seen.read(ctx, c) })
	}

	ask := func() {
		for _, c := range conns {
			for _, q := range seen.questions() {
				// from the mDNS port a plain question; from any other, one asking to be answered directly
				direct := c.LocalAddr().(*net.UDPAddr).Port != mdnsGroup.Port //nolint:errcheck,forcetypeassert // a UDP conn has a UDP address
				if msg, err := query(q.name, q.kind, direct); err == nil {
					_, _ = c.WriteToUDP(msg, mdnsGroup) // one interface that cannot send is not the search failing
				}
			}
		}
	}

	ask()
	again := time.NewTimer(wait / 3)
	defer again.Stop()
	select {
	case <-again.C:
		ask() // and for whatever the first answers left out
	case <-ctx.Done():
	}
	<-ctx.Done()
	for _, c := range conns {
		_ = c.SetReadDeadline(time.Now()) // wake the readers
	}
	wg.Wait()
	sys.Wait()

	found := merge(seen.found(), system)
	if len(conns) == 0 && len(found) == 0 {
		return nil, errors.New("no network to search: nothing to send a query on")
	}

	return found, nil
}

// merge puts two lists of controllers together by name, filling in what
// either left out.
func merge(a, b []Found) []Found {
	out := slices.Clone(a)
	for _, f := range b {
		i := slices.IndexFunc(out, func(o Found) bool { return o.Name == f.Name })
		if i < 0 {
			out = append(out, f)
			continue
		}
		o := &out[i]
		if o.Host == "" {
			o.Host, o.Port = f.Host, f.Port
		}
		for _, addr := range f.Addrs {
			if !slices.Contains(o.Addrs, addr) {
				o.Addrs = append(o.Addrs, addr)
			}
		}
		o.Model, o.Firmware, o.ID = cmp.Or(o.Model, f.Model), cmp.Or(o.Firmware, f.Firmware), cmp.Or(o.ID, f.ID)
	}
	slices.SortFunc(out, func(x, y Found) int { return strings.Compare(x.Name, y.Name) })

	return out
}

// listen opens the sockets Browse asks on: one joined to the mDNS group on
// each network interface that can multicast, and one on a port of its own.
func listen() []*net.UDPConn {
	var conns []*net.UDPConn

	ifaces, _ := net.Interfaces() // none is the same as an error: there is nothing to join
	for i := range ifaces {
		ifi := &ifaces[i]
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if c, err := net.ListenMulticastUDP("udp4", ifi, mdnsGroup); err == nil {
			conns = append(conns, c)
		}
	}
	if c, err := net.ListenUDP("udp4", &net.UDPAddr{}); err == nil {
		conns = append(conns, c)
	}

	return conns
}

// query is one mDNS question. direct sets the bit that asks for the answer to
// be sent back to the asker rather than to everyone.
func query(name string, kind dnsmessage.Type, direct bool) ([]byte, error) {
	n, err := dnsmessage.NewName(name)
	if err != nil {
		return nil, err
	}
	class := dnsmessage.ClassINET
	if direct {
		class |= 1 << 15
	}
	msg := dnsmessage.Message{Questions: []dnsmessage.Question{{Name: n, Type: kind, Class: class}}}

	return msg.Pack()
}

// records is what the answers have said so far.
type records struct {
	mu        sync.Mutex
	instances map[string]bool         // "Light Panels 53:A6:3C._nanoleafapi._tcp.local."
	targets   map[string]target       // instance to where it is served from
	text      map[string][]string     // instance to its TXT strings
	addrs     map[string][]netip.Addr // host name to addresses
}

type target struct {
	host string
	port int
}

type question struct {
	name string
	kind dnsmessage.Type
}

func newRecords() *records {
	return &records{instances: map[string]bool{}, targets: map[string]target{}, text: map[string][]string{}, addrs: map[string][]netip.Addr{}}
}

// read takes answers off one socket until the search is over.
func (r *records) read(ctx context.Context, c *net.UDPConn) {
	buf := make([]byte, 9000)
	for ctx.Err() == nil {
		n, _, err := c.ReadFromUDP(buf)
		if err != nil {
			return // the deadline Browse sets to end the search, or a closed socket
		}
		r.add(buf[:n])
	}
}

// add reads one packet. Anything in it that is not about controllers, and any
// packet that is not DNS at all, is passed over: the mDNS group carries every
// device's chatter.
func (r *records) add(packet []byte) {
	var p dnsmessage.Parser
	if _, err := p.Start(packet); err != nil {
		return
	}
	if p.SkipAllQuestions() != nil {
		return
	}

	// answers, then authorities and additionals: a responder puts the address in any of them
	var all []dnsmessage.Resource
	for _, section := range []func() ([]dnsmessage.Resource, error){p.AllAnswers, p.AllAuthorities, p.AllAdditionals} {
		rs, err := section()
		all = append(all, rs...)
		if err != nil {
			break
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, res := range all {
		name := strings.ToLower(res.Header.Name.String())
		switch body := res.Body.(type) {
		case *dnsmessage.PTRResource:
			if name == Service {
				r.instances[body.PTR.String()] = true
			}
		case *dnsmessage.SRVResource:
			r.targets[res.Header.Name.String()] = target{host: body.Target.String(), port: int(body.Port)}
		case *dnsmessage.TXTResource:
			r.text[res.Header.Name.String()] = body.TXT
		case *dnsmessage.AResource:
			r.addAddr(name, netip.AddrFrom4(body.A))
		case *dnsmessage.AAAAResource:
			r.addAddr(name, netip.AddrFrom16(body.AAAA))
		}
	}
}

func (r *records) addAddr(host string, addr netip.Addr) {
	if !slices.Contains(r.addrs[host], addr) {
		r.addrs[host] = append(r.addrs[host], addr)
	}
}

// questions is what is still worth asking: who is there, and for each one
// heard of, whatever its answer left out.
func (r *records) questions() []question {
	r.mu.Lock()
	defer r.mu.Unlock()

	qs := []question{{Service, dnsmessage.TypePTR}}
	for inst := range r.instances {
		t, ok := r.targets[inst]
		if !ok {
			qs = append(qs, question{inst, dnsmessage.TypeSRV})
		}
		if _, noted := r.text[inst]; !noted {
			qs = append(qs, question{inst, dnsmessage.TypeTXT})
		}
		if ok && len(r.addrs[strings.ToLower(t.host)]) == 0 {
			qs = append(qs, question{t.host, dnsmessage.TypeA})
		}
	}

	return qs
}

// found puts the records together into controllers.
func (r *records) found() []Found {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Found, 0, len(r.instances))
	for inst := range r.instances {
		f := Found{Name: instanceName(inst), Port: DefaultPort}
		if t, ok := r.targets[inst]; ok {
			f.Host = strings.TrimSuffix(t.host, ".")
			if t.port != 0 {
				f.Port = t.port
			}
			f.Addrs = slices.Clone(r.addrs[strings.ToLower(t.host)])
			slices.SortFunc(f.Addrs, func(a, b netip.Addr) int {
				if a.Is4() != b.Is4() {
					return cmp.Compare(b2i(b.Is4()), b2i(a.Is4())) // IPv4 first: it is what people type
				}
				return a.Compare(b)
			})
		}
		for _, kv := range r.text[inst] {
			switch k, v, _ := strings.Cut(kv, "="); strings.ToLower(k) {
			case "md":
				f.Model = v
			case "srcvers":
				f.Firmware = v
			case "id":
				f.ID = v
			}
		}
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b Found) int { return strings.Compare(a.Name, b.Name) })

	return out
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// instanceName is the name in front of the service type, with the escapes
// DNS puts on spaces and dots taken off: "Light\032Panels\03253:A6:3C".
func instanceName(instance string) string {
	label := instance
	if i := strings.Index(strings.ToLower(instance), "."+Service); i >= 0 {
		label = instance[:i]
	}

	var b strings.Builder
	for i := 0; i < len(label); i++ {
		if label[i] != '\\' || i+1 >= len(label) {
			b.WriteByte(label[i])
			continue
		}
		// \DDD is a byte in decimal, \X is X
		if i+3 < len(label) {
			if n, err := strconv.ParseUint(label[i+1:i+4], 10, 8); err == nil {
				b.WriteByte(byte(n)) // parsed as eight bits, so it fits
				i += 3
				continue
			}
		}
		i++
		b.WriteByte(label[i])
	}

	return b.String()
}

// Scan knocks on every address of a subnet and returns the ones a controller
// answers at. It is for networks mDNS does not cross: it cannot learn a
// controller's name, only that one is there. timeout is how long each address
// gets.
func Scan(ctx context.Context, prefix netip.Prefix, port int, timeout time.Duration) ([]Found, error) {
	prefix = prefix.Masked()
	if !prefix.Addr().Is4() {
		return nil, fmt.Errorf("%s: only IPv4 subnets can be scanned", prefix)
	}
	hosts := 1 << (32 - prefix.Bits())
	if hosts > maxScan {
		return nil, fmt.Errorf("%s is %d addresses: scan a /%d or smaller", prefix, hosts, 32-12)
	}
	if port == 0 {
		port = DefaultPort
	}

	client := &http.Client{
		Timeout:       timeout,
		Transport:     &http.Transport{DisableKeepAlives: true, DialContext: (&net.Dialer{Timeout: timeout}).DialContext},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()

	var (
		mu    sync.Mutex
		found []Found
		wg    sync.WaitGroup
	)
	slots := make(chan struct{}, 64)
	for addr := prefix.Addr(); prefix.Contains(addr) && ctx.Err() == nil; addr = addr.Next() {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			if isController(ctx, client, addr, port) {
				mu.Lock()
				defer mu.Unlock()
				found = append(found, Found{Addrs: []netip.Addr{addr}, Port: port})
			}
		})
	}
	wg.Wait()
	slices.SortFunc(found, func(a, b Found) int { return a.Addrs[0].Compare(b.Addrs[0]) })

	return found, ctx.Err()
}

// isController asks an address for the API with no token. A controller
// refuses with a 401 and nothing else; anything else on that port is
// something else.
func isController(ctx context.Context, client *http.Client, addr netip.Addr, port int) bool {
	u := "http://" + net.JoinHostPort(addr.String(), strconv.Itoa(port)) + "/api/v1/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return false
	}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = res.Body.Close() }()

	return res.StatusCode == http.StatusUnauthorized
}
