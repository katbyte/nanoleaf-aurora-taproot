package discover

import (
	"bufio"
	"context"
	"net/netip"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// On a Mac, answers to a search of our own often never arrive: the firewall
// drops what comes in for a program it has not been told to trust, without a
// word, and a program run from a terminal is rarely one it has. The system's
// own discovery service is not held back that way, and dns-sd, which ships
// with macOS, asks it. So on a Mac both are tried and what either finds is
// kept.

// how long one lookup of a controller's details or address may take. One
// that works is over in a moment; this is for the system having to go and
// ask again because what it knew has gone stale, which takes seconds.
const lookupFor = 5 * time.Second

var (
	// 13:12:18.415  Light\032Panels\03252:56:C3._nanoleafapi._tcp.local. can be reached at Light-Panels-52-56-C3.local.:16021 (interface 14)
	reachedAt = regexp.MustCompile(` can be reached at (\S+?)\.?:(\d+)`)
	// 13:12:20.425  Add  40000002      14  Light-Panels-52-56-C3.local.           10.0.5.182                                   120
	addressOf = regexp.MustCompile(`\sAdd\s.*\s(\d+\.\d+\.\d+\.\d+)\s`)
)

// systemWay says how systemBrowse looks, or nothing when it cannot.
func systemWay() string {
	if _, err := exec.LookPath("dns-sd"); err != nil {
		return ""
	}
	return "asking macOS's own discovery service (dns-sd), since a firewall here often keeps the answers to the first from arriving"
}

// systemBrowse asks the system's discovery service for controllers until ctx
// ends. It returns nothing, rather than an error, when dns-sd is not there or
// says nothing: it is the second of two ways of looking.
func systemBrowse(ctx context.Context) []Found {
	var (
		mu    sync.Mutex
		found []Found
		wg    sync.WaitGroup
		seen  = map[string]bool{}
	)

	// one line per controller as it is heard of:
	// 13:11:03.934  Add        3  14 local.               _nanoleafapi._tcp.   Light Panels 53:A6:3C
	const marker = "_nanoleafapi._tcp."
	watch(ctx, func(line string) {
		before, after, ok := strings.Cut(line, marker)
		if !ok || !strings.Contains(before, " Add ") {
			return
		}
		name := strings.TrimSpace(after)
		// a name that reads as an option would be taken as one by dns-sd
		if name == "" || strings.HasPrefix(name, "-") || seen[name] {
			return
		}
		seen[name] = true
		wg.Go(func() {
			f := resolve(ctx, name)
			mu.Lock()
			defer mu.Unlock()
			found = append(found, f)
		})
	}, "-B", "_nanoleafapi._tcp", "local.")
	wg.Wait()

	return found
}

// resolve asks where a controller is served from, what it says about itself,
// and what address that is.
func resolve(ctx context.Context, name string) Found {
	f := Found{Name: name, Port: DefaultPort, Via: []string{ViaSystem}}

	// the lookups get their own time: the search ending must not cut short the one for a controller heard of at its end
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lookupFor)
	defer cancel()
	located := false
	watch(lctx, func(line string) {
		if m := reachedAt.FindStringSubmatch(line); m != nil {
			f.Host = m[1]
			if port, err := strconv.Atoi(m[2]); err == nil {
				f.Port = port
			}
			located = true
			return
		}
		// the line after it is the controller's own notes: id=... md=NL22 srcvers=5.2.1
		if !located {
			return
		}
		for kv := range strings.FieldsSeq(line) {
			switch k, v, _ := strings.Cut(kv, "="); k {
			case "md":
				f.Model = v
			case "srcvers":
				f.Firmware = v
			case "id":
				f.ID = v
			}
		}
		if f.Model != "" || f.ID != "" {
			cancel() // everything it has to say
		}
	}, "-L", name, "_nanoleafapi._tcp", "local.")
	if f.Host == "" {
		return f
	}

	actx, stop := context.WithTimeout(context.WithoutCancel(ctx), lookupFor)
	defer stop()
	watch(actx, func(line string) {
		if m := addressOf.FindStringSubmatch(line); m != nil {
			if addr, err := netip.ParseAddr(m[1]); err == nil {
				f.Addrs = append(f.Addrs, addr)
				stop()
			}
		}
	}, "-G", "v4", f.Host)

	return f
}

// watch runs dns-sd and hands over each line it prints until ctx ends, which
// is the only way it stops: it reports what it hears for as long as it is
// left running.
func watch(ctx context.Context, line func(string), args ...string) {
	cmd := exec.CommandContext(ctx, "dns-sd", args...) //nolint:gosec // dns-sd by name with no shell; the one argument from the network is checked by the caller
	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if cmd.Start() != nil {
		return
	}
	lines := bufio.NewScanner(out)
	for lines.Scan() {
		line(lines.Text())
	}
	_ = cmd.Wait() // it only ever ends by being stopped
}
