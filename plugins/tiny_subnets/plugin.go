/*
This plugin supports assignment with  /30 addresses for small, segmented subnets

*/
package tiny_subnets

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
	"strings"

	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/insomniacslk/dhcp/dhcpv4"
)

import (
        "github.com/gorilla/mux"
        "net/http"
				"encoding/json"
)

var UNIX_PLUGIN_LISTENER = "/state/dhcp/tinysubnets_plugin"

type AbstractDHCPRequest struct {
	Identifier string
}

// When an abstract device is added:
// request an IP address, returning a Record on success
// This allows decoupling DHCP records from MAC addresses/UDP DHCP packets.
func (p *PluginState) abstractDHCP(w http.ResponseWriter, r *http.Request) {
	p.Lock()
	defer p.Unlock()

	req := AbstractDHCPRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
					http.Error(w, err.Error(), 400)
					return
	}

	if req.Identifier == "" || (strings.TrimSpace(req.Identifier) != req.Identifier) ||
		strings.Contains(req.Identifier, " ") ||
		strings.Contains(req.Identifier, "\n")	{
		http.Error(w, "Invalid Identifier", 400)
		return
	}

	record, success := p.requestRecord(req.Identifier, 0)
	if !success {
		http.Error(w, "Failed to get IP", 400)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(record)
}


func logRequest(handler http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          fmt.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
          handler.ServeHTTP(w, r)
  })
}



var log = logger.GetLogger("plugins/tiny_subnets")

// Plugin wraps plugin registration information
var Plugin = plugins.Plugin{
	Name:   "tiny_subnets",
	Setup4: setupPoint,
}

//Record holds an IP lease record
type Record struct {
	IP      net.IP
	RouterIP	net.IP
	expires time.Time
}

var (
	ipRangeStart net.IP
	ipRangeEnd net.IP
)

// PluginState is the data held by an instance of the range plugin
type PluginState struct {
	// Rough lock for the whole plugin, we'll get better performance once we use leasestorage
	sync.Mutex
	// Recordsv4 holds a MAC -> IP address and lease time mapping
	Recordsv4 map[string]*Record
	IPTaken map[uint32]bool
	LeaseTime time.Duration
	leasefile *os.File
}

func (p *PluginState) requestRecord(clientAddr string, subMask uint32) (*Record, bool) {

	record, ok := p.Recordsv4[clientAddr]

	if !ok {
		// Allocating new address since there isn't one allocated
		log.Printf("Client address %s is new, leasing new IPv4 address", clientAddr)

		if (subMask != 0) {
			// Expecting a /30
			slash30 := binary.BigEndian.Uint32(net.IPv4Mask(255,255,255,252))

			if subMask != slash30 {
				log.Errorf("Only /30 (255.255.255.252) is currently supported")
				return &Record{}, false
			}
		}

		//run from start until end, incrementing by 4
		ipStart := binary.BigEndian.Uint32(ipRangeStart.To4())
		ipEnd := binary.BigEndian.Uint32(ipRangeEnd.To4())

		var routerIP net.IP
		var ip net.IP

		for u32_ip := ipStart; u32_ip + 3 < ipEnd; u32_ip += 4 {
				u := u32_ip + 1
				routerIP = net.IPv4(byte(u>>24), byte(u>>16), byte(u>>8), byte(u))

				u = u32_ip + 2
				ip = net.IPv4(byte(u>>24), byte(u>>16), byte(u>>8), byte(u))

				if (p.IPTaken[u]) {
					continue
				} else {
					//found an entry to use
					break;
				}
		}

		if ip == nil {
			log.Errorf("Could not allocate IP for ClientAddr %s: ran out", clientAddr)
			return &Record{}, false
		}

		rec := Record{
			IP:      ip.To4(),
			RouterIP: routerIP,
			expires: time.Now().Add(p.LeaseTime),
		}

		err := p.saveIPAddress(clientAddr, &rec)
		if err != nil {
			log.Errorf("SaveIPAddress for MAC %s failed: %v", clientAddr, err)
			return &Record{}, false
		}
		p.Recordsv4[clientAddr] = &rec
		p.IPTaken[ binary.BigEndian.Uint32(ip.To4()) ] = true
		record = &rec
	} else {
		// Ensure we extend the existing lease at least past when the one we're giving expires
		if record.expires.Before(time.Now().Add(p.LeaseTime)) {
			record.expires = time.Now().Add(p.LeaseTime).Round(time.Second)
			err := p.saveIPAddress(clientAddr, record)
			if err != nil {
				log.Errorf("Could not persist lease for ClientAddr %s: %v", clientAddr, err)
				return &Record{}, false
			}
		}

		//calculate the router ip  from the recored IP. it's just -1
		u := binary.BigEndian.Uint32(record.IP.To4())
		u = u - 1
		record.RouterIP = net.IPv4(byte(u>>24), byte(u>>16), byte(u>>8), byte(u))

	}

	return record, true
}

// Handler4 handles DHCPv4 packets for the range plugin
func (p *PluginState) Handler4(state *handler.PropagateState, req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	p.Lock()
	defer p.Unlock()

	subMask := binary.BigEndian.Uint32(resp.Options[uint8(dhcpv4.OptionSubnetMask)])

	record, success := p.requestRecord(req.ClientHWAddr.String(), subMask)
	if !success {
		return nil, true
	}

	resp.YourIPAddr = record.IP

	resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(p.LeaseTime.Round(time.Second)))
	resp.Options.Update(dhcpv4.OptRouter(record.RouterIP))
	//resp.Options.Updaet()

	log.Printf("found IP address %s for ClientAddr %s", record.IP, req.ClientHWAddr.String())
	return resp, false
}

func setupPoint(args ...string) (handler.Handler4, error) {
	var (
		err error
		p   PluginState
	)

	if len(args) < 4 {
		return nil, fmt.Errorf("invalid number of arguments, want: 4 (file name, start IP, end IP, lease time), got: %d", len(args))
	}
	filename := args[0]
	if filename == "" {
		return nil, errors.New("file name cannot be empty")
	}
	ipRangeStart = net.ParseIP(args[1])
	if ipRangeStart.To4() == nil {
		return nil, fmt.Errorf("invalid IPv4 address: %v", args[1])
	}
	ipRangeEnd = net.ParseIP(args[2])
	if ipRangeEnd.To4() == nil {
		return nil, fmt.Errorf("invalid IPv4 address: %v", args[2])
	}
	if binary.BigEndian.Uint32(ipRangeStart.To4()) >= binary.BigEndian.Uint32(ipRangeEnd.To4()) {
		return nil, errors.New("start of IP range has to be lower than the end of an IP range")
	}

	p.LeaseTime, err = time.ParseDuration(args[3])
	if err != nil {
		return nil, fmt.Errorf("invalid lease duration: %v", args[3])
	}

	p.Recordsv4, err = loadRecordsFromFile(filename)
	if err != nil {
		return nil, fmt.Errorf("could not load records from file: %v", err)
	}

	p.IPTaken = map[uint32]bool {}
	for _, record := range p.Recordsv4 {
		p.IPTaken[binary.BigEndian.Uint32(record.IP.To4())] = true
	}

	log.Printf("Loaded %d DHCPv4 leases from %s", len(p.Recordsv4), filename)

	if err := p.registerBackingFile(filename); err != nil {
		return nil, fmt.Errorf("could not setup lease storage: %w", err)
	}

	unix_plugin_router := mux.NewRouter().StrictSlash(true)
	unix_plugin_router.HandleFunc("/DHCPRequest", p.abstractDHCP).Methods("PUT")
	os.Remove(UNIX_PLUGIN_LISTENER)
	unixPluginListener, err := net.Listen("unix", UNIX_PLUGIN_LISTENER)
	if err != nil {
					panic(err)
	}
	pluginServer := http.Server{Handler: logRequest(unix_plugin_router)}
	go pluginServer.Serve(unixPluginListener)

	return p.Handler4, nil
}
