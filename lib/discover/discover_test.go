package discover

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

const (
	instance = `Light\032Panels\03253:A6:3C._nanoleafapi._tcp.local.`
	hostName = "Light-Panels-53-A6-3C.local."
)

// answer builds an mDNS answer packet out of the records given.
func answer(t *testing.T, records ...dnsmessage.Resource) []byte {
	t.Helper()

	msg := dnsmessage.Message{Answers: records}
	msg.Response, msg.Authoritative = true, true
	packet, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}

	return packet
}

func header(t *testing.T, name string, kind dnsmessage.Type) dnsmessage.ResourceHeader {
	t.Helper()

	n, err := dnsmessage.NewName(name)
	if err != nil {
		t.Fatal(err)
	}

	return dnsmessage.ResourceHeader{Name: n, Type: kind, Class: dnsmessage.ClassINET, TTL: 120}
}

func ptr(t *testing.T, service, inst string) dnsmessage.Resource {
	t.Helper()
	return dnsmessage.Resource{Header: header(t, service, dnsmessage.TypePTR), Body: &dnsmessage.PTRResource{PTR: dnsmessage.MustNewName(inst)}}
}

func srv(t *testing.T, inst, target string, port uint16) dnsmessage.Resource {
	t.Helper()
	return dnsmessage.Resource{Header: header(t, inst, dnsmessage.TypeSRV), Body: &dnsmessage.SRVResource{Target: dnsmessage.MustNewName(target), Port: port}}
}

func txt(t *testing.T, inst string, kv ...string) dnsmessage.Resource {
	t.Helper()
	return dnsmessage.Resource{Header: header(t, inst, dnsmessage.TypeTXT), Body: &dnsmessage.TXTResource{TXT: kv}}
}

func a(t *testing.T, host string, ip [4]byte) dnsmessage.Resource {
	t.Helper()
	return dnsmessage.Resource{Header: header(t, host, dnsmessage.TypeA), Body: &dnsmessage.AResource{A: ip}}
}

func TestRecordsFromOnePacket(t *testing.T) {
	t.Parallel()

	r := newRecords()
	r.add(answer(t,
		ptr(t, Service, instance),
		srv(t, instance, hostName, 16021),
		txt(t, instance, "id=67:83:74:31:35:CE", "md=NL22", "srcvers=5.2.1"),
		a(t, hostName, [4]byte{10, 0, 5, 183}),
		dnsmessage.Resource{Header: header(t, hostName, dnsmessage.TypeAAAA), Body: &dnsmessage.AAAAResource{AAAA: [16]byte{0xfe, 0x80, 15: 1}}},
	))

	found := r.found()
	if len(found) != 1 {
		t.Fatalf("found %d controllers: %+v", len(found), found)
	}
	f := found[0]
	if f.Name != "Light Panels 53:A6:3C" || f.Host != "Light-Panels-53-A6-3C.local" || f.Port != 16021 {
		t.Errorf("who and where: %+v", f)
	}
	if f.Model != "NL22" || f.Firmware != "5.2.1" || f.ID != "67:83:74:31:35:CE" {
		t.Errorf("what it says of itself: %+v", f)
	}
	// IPv4 first: it is the address a person would type
	if len(f.Addrs) != 2 || f.Addrs[0] != netip.MustParseAddr("10.0.5.183") || !f.Addrs[1].Is6() {
		t.Errorf("addresses: %v", f.Addrs)
	}
	if f.Address() != "10.0.5.183" {
		t.Errorf("Address: %q", f.Address())
	}
	if !slices.Equal(f.Via, []string{ViaAnnouncement}) {
		t.Errorf("found by %v", f.Via)
	}
}

// Ways is for telling a person what a search is doing, so it always has
// something to say, and says which service is being looked for.
func TestWays(t *testing.T) {
	t.Parallel()

	ways := Ways()
	if len(ways) == 0 || !strings.Contains(ways[0], "mDNS, _nanoleafapi._tcp") {
		t.Errorf("Ways: %q", ways)
	}
	for _, way := range ways {
		if way == "" || strings.HasSuffix(way, " on ") {
			t.Errorf("a way with nothing in it: %q", way)
		}
	}
}

// A responder may answer the first question with a name and nothing else,
// and the rest only when asked.
func TestRecordsPieceByPiece(t *testing.T) {
	t.Parallel()

	r := newRecords()
	if qs := r.questions(); len(qs) != 1 || qs[0].name != Service || qs[0].kind != dnsmessage.TypePTR {
		t.Fatalf("to begin with there is one thing to ask: %+v", qs)
	}

	r.add(answer(t, ptr(t, Service, instance)))
	kinds := func() []dnsmessage.Type {
		qs := r.questions()
		out := make([]dnsmessage.Type, 0, len(qs))
		for _, q := range qs {
			out = append(out, q.kind)
		}
		slices.Sort(out)
		return out
	}
	if got := kinds(); !slices.Equal(got, []dnsmessage.Type{dnsmessage.TypePTR, dnsmessage.TypeTXT, dnsmessage.TypeSRV}) {
		t.Errorf("heard of by name only, still to ask: %v", got)
	}
	if f := r.found(); len(f) != 1 || f[0].Address() != "" || f[0].Port != DefaultPort {
		t.Errorf("a controller heard of but not located: %+v", f)
	}

	r.add(answer(t, srv(t, instance, hostName, 8080), txt(t, instance, "md=NL22")))
	if got := kinds(); !slices.Equal(got, []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypePTR}) {
		t.Errorf("located but with no address, still to ask: %v", got)
	}
	// no address yet: the host name will do, with the port since it is not the usual one
	if f := r.found(); f[0].Address() != "Light-Panels-53-A6-3C.local:8080" {
		t.Errorf("Address from a host name: %q", f[0].Address())
	}

	r.add(answer(t, a(t, hostName, [4]byte{10, 0, 5, 183})))
	r.add(answer(t, a(t, hostName, [4]byte{10, 0, 5, 183}))) // heard twice, listed once
	if got := kinds(); !slices.Equal(got, []dnsmessage.Type{dnsmessage.TypePTR}) {
		t.Errorf("with everything known, still to ask: %v", got)
	}
	if f := r.found(); len(f[0].Addrs) != 1 || f[0].Address() != "10.0.5.183:8080" {
		t.Errorf("Address: %q from %v", f[0].Address(), f[0].Addrs)
	}
}

func TestRecordsPassOverEverythingElse(t *testing.T) {
	t.Parallel()

	r := newRecords()
	r.add([]byte("not dns at all"))
	r.add(nil)
	// a printer announcing itself on the same group
	r.add(answer(t,
		ptr(t, "_ipp._tcp.local.", "Printer._ipp._tcp.local."),
		srv(t, "Printer._ipp._tcp.local.", "printer.local.", 631),
		a(t, "printer.local.", [4]byte{10, 0, 0, 9}),
	))
	if f := r.found(); len(f) != 0 {
		t.Errorf("found %+v among other devices' chatter", f)
	}

	// two controllers come back sorted by name
	r.add(answer(t, ptr(t, Service, `Light\032Panels\03253:A6:3C._nanoleafapi._tcp.local.`), ptr(t, Service, `Light\032Panels\03251:1D:7D._nanoleafapi._tcp.local.`)))
	if f := r.found(); len(f) != 2 || f[0].Name != "Light Panels 51:1D:7D" || f[1].Name != "Light Panels 53:A6:3C" {
		t.Errorf("found %+v", f)
	}
}

func TestInstanceName(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		`Light\032Panels\03253:A6:3C._nanoleafapi._tcp.local.`: "Light Panels 53:A6:3C",
		`Dot\.ted._nanoleafapi._tcp.local.`:                    "Dot.ted",
		`Plain._NanoleafAPI._tcp.local.`:                       "Plain",
		`Trailing\`:                                            `Trailing\`,
		`Odd\9x._nanoleafapi._tcp.local.`:                      "Odd9x",
		"no service here":                                      "no service here",
	} {
		if got := instanceName(in); got != want {
			t.Errorf("instanceName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQuery(t *testing.T) {
	t.Parallel()

	for _, direct := range []bool{false, true} {
		packet, err := query(Service, dnsmessage.TypePTR, direct)
		if err != nil {
			t.Fatal(err)
		}
		var p dnsmessage.Parser
		if _, err := p.Start(packet); err != nil {
			t.Fatal(err)
		}
		q, err := p.Question()
		if err != nil || q.Name.String() != Service || q.Type != dnsmessage.TypePTR {
			t.Fatalf("question: %+v, %v", q, err)
		}
		// the top bit of the class asks for the answer to be sent straight back
		if got := q.Class&(1<<15) != 0; got != direct {
			t.Errorf("direct=%v: class %#x", direct, uint16(q.Class))
		}
	}
	if _, err := query(strings.Repeat("x", 300)+".local.", dnsmessage.TypePTR, false); err == nil {
		t.Error("a name too long for DNS should be an error")
	}
}

func TestMerge(t *testing.T) {
	t.Parallel()

	ours := []Found{
		{Name: "B", Port: 16021, Via: []string{ViaAnnouncement}},
		{Name: "A", Host: "a.local", Port: 16021, Addrs: []netip.Addr{netip.MustParseAddr("10.0.5.1")}, Model: "NL22", Via: []string{ViaAnnouncement}},
	}
	theirs := []Found{
		{Name: "B", Host: "b.local", Port: 16021, Addrs: []netip.Addr{netip.MustParseAddr("10.0.5.2")}, Model: "NL22", Firmware: "5.3.2", ID: "id", Via: []string{ViaSystem}},
		{Name: "A", Host: "other.local", Addrs: []netip.Addr{netip.MustParseAddr("10.0.5.1")}, Firmware: "5.2.1", Via: []string{ViaSystem}},
		{Name: "C", Port: 16021, Via: []string{ViaSystem}},
	}
	got := merge(ours, theirs)
	if len(got) != 3 || got[0].Name != "A" || got[1].Name != "B" || got[2].Name != "C" {
		t.Fatalf("merged: %+v", got)
	}
	// what one way of looking found is kept, and filled in from the other
	if a := got[0]; a.Host != "a.local" || len(a.Addrs) != 1 || a.Model != "NL22" || a.Firmware != "5.2.1" {
		t.Errorf("A: %+v", a)
	}
	if b := got[1]; b.Host != "b.local" || b.Address() != "10.0.5.2" || b.Firmware != "5.3.2" || b.ID != "id" {
		t.Errorf("B: %+v", b)
	}
	// one found both ways says so, and one found one way says which
	if !slices.Equal(got[0].Via, []string{ViaAnnouncement, ViaSystem}) || !slices.Equal(got[2].Via, []string{ViaSystem}) {
		t.Errorf("found by: A %v, C %v", got[0].Via, got[2].Via)
	}
	if len(ours[0].Addrs) != 0 || len(ours[0].Via) != 1 {
		t.Error("merge changed its argument")
	}
}

// Browse on a machine with no controllers, or no network at all, comes back
// when its time is up and not before.
func TestBrowseTakesItsTime(t *testing.T) {
	t.Parallel()

	start := time.Now()
	_, _ = Browse(t.Context(), 300*time.Millisecond) // what is on the network is not this test's to say
	if time.Since(start) < 250*time.Millisecond || time.Since(start) > 10*time.Second {
		t.Errorf("Browse for 300ms took %s", time.Since(start))
	}
}

func TestScan(t *testing.T) {
	t.Parallel()

	// a controller refuses a request with no token; anything else on the port is something else
	serve := func(status int) (netip.Addr, int) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/" {
				t.Errorf("knocked at %s", r.URL.Path)
			}
			w.WriteHeader(status)
		}))
		t.Cleanup(srv.Close)
		host, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
		n, _ := strconv.Atoi(port)
		return netip.MustParseAddr(host), n
	}

	addr, port := serve(http.StatusUnauthorized)
	found, err := Scan(t.Context(), netip.PrefixFrom(addr, 32), port, 2*time.Second)
	if err != nil || len(found) != 1 || found[0].Addrs[0] != addr || found[0].Port != port || found[0].Name != "" || !slices.Equal(found[0].Via, []string{ViaScan}) {
		t.Errorf("a controller: %+v, %v", found, err)
	}
	if want := net.JoinHostPort(addr.String(), strconv.Itoa(port)); found[0].Address() != want {
		t.Errorf("Address: %q, want %q", found[0].Address(), want)
	}

	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusFound} {
		addr, port := serve(status)
		if found, err := Scan(t.Context(), netip.PrefixFrom(addr, 32), port, 2*time.Second); err != nil || len(found) != 0 {
			t.Errorf("something answering %d is not a controller: %+v, %v", status, found, err)
		}
	}

	// nothing listening
	if found, err := Scan(t.Context(), netip.MustParsePrefix("127.0.0.1/32"), 1, 500*time.Millisecond); err != nil || len(found) != 0 {
		t.Errorf("a closed port: %+v, %v", found, err)
	}
}

func TestScanRefuses(t *testing.T) {
	t.Parallel()

	if _, err := Scan(t.Context(), netip.MustParsePrefix("10.0.0.0/8"), 0, time.Second); err == nil || !strings.Contains(err.Error(), "scan a /20 or smaller") {
		t.Errorf("a whole /8: %v", err)
	}
	if _, err := Scan(t.Context(), netip.MustParsePrefix("fe80::/120"), 0, time.Second); err == nil || !strings.Contains(err.Error(), "IPv4") {
		t.Errorf("an IPv6 subnet: %v", err)
	}
}
