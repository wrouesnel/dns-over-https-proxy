/*
dns-over-https-proxy is a DNS proxy server to serve DNS via the Google
HTTPS DNS endpoint.

Usage:
go run dns_reverse_proxy.go -debug=true -address=127.0.0.1:8500 -log.level=debug
*/
package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"encoding/json"
	"net"
	"net/http"

	"strings"

	"fmt"

	"github.com/miekg/dns"
	"github.com/wrouesnel/go.log"
)

var (
	address = flag.String("address", ":53", "Address to listen to (TCP and UDP)")

	defaultServer = flag.String("default", "https://dns.google.com/resolve",
		"DNS-over-HTTPS service endpoint")

	prefixServer = flag.String("primary-dns", "",
		"If set all DNS queries are attempted against this DNS server first before trying HTTPS")

	suffixServer = flag.String("fallback-dns", "",
		"If set all failed (i.e. NXDOMAIN and others) DNS queries are attempted against this DNS server after HTTPS fails.") // nolint: lll

	fallthroughStatuses = flag.String("fallthrough-statuses", "NXDOMAIN",
		"Comma-separated list of statuses which should cause server fallthrough")
	neverDefault = flag.String("no-fallthrough", "",
		"Comma-separated list of suffixes which will not be allowed to fallthrough (most useful with prefix DNS")

	//routeList = flag.String("route", "",
	//	"List of routes where to send queries (subdomain=IP:port)")
	//routes map[string]string

	//allowTransfer = flag.String("allow-transfer", "",
	//	"List of IPs allowed to transfer (AXFR/IXFR)")

	debug = flag.Bool("debug", false, "Verbose debugging")

	//transferIPs []string
)

// DNSResponseJSON is a rough translation of the Google DNS over HTTP API as it currently exists.
type DNSResponseJSON struct {
	Status           int32         `json:"Status,omitempty"`
	TC               bool          `json:"TC,omitempty"`
	RD               bool          `json:"RD,omitempty"`
	RA               bool          `json:"RA,omitempty"`
	AD               bool          `json:"AD,omitempty"`
	CD               bool          `json:"CD,omitempty"`
	Question         []DNSQuestion `json:"Question,omitempty"`
	Answer           []DNSRR       `json:"Answer,omitempty"`
	Authority        []DNSRR       `json:"Authority,omitempty"`
	Additional       []DNSRR       `json:"Additional,omitempty"`
	EdnsClientSubnet string        `json:"edns_client_subnet,omitempty"`
	Comment          string        `json:"Comment,omitempty"`
}

// DNSQuestion is the JSON encoding of a DNS request
type DNSQuestion struct {
	Name string `json:"name,omitempty"`
	Type int32  `json:"type,omitempty"`
}

// DNSRR is the JSON encoding of an RRset as returned by Google.
type DNSRR struct {
	Name string `json:"name,omitempty"`
	Type int32  `json:"type,omitempty"`
	TTL  int32  `json:"TTL,omitempty"`
	Data string `json:"data,omitempty"`
}

// NewRR initializes a new RRGeneric from a DNSRR
func NewRR(a DNSRR) dns.RR {
	var rr dns.RR

	// Build an RR header
	rrhdr := dns.RR_Header{
		Name:     a.Name,
		Rrtype:   uint16(a.Type),
		Class:    dns.ClassINET,
		Ttl:      uint32(a.TTL),
		Rdlength: uint16(len(a.Data)),
	}

	constructor, ok := dns.TypeToRR[uint16(a.Type)]
	if ok {
		// Construct a new RR
		rr = constructor()
		*(rr.Header()) = rrhdr
		switch v := rr.(type) {
		case *dns.A:
			v.A = net.ParseIP(a.Data)
		case *dns.AAAA:
			v.AAAA = net.ParseIP(a.Data)
		}
	} else {
		rr = dns.RR(&dns.RFC3597{
			Hdr:   rrhdr,
			Rdata: a.Data,
		})
	}
	return rr
}

func main() {
	flag.Parse()
	if *defaultServer == "" {
		log.Fatal("-default is required")
	}
	//transferIPs = strings.Split(*allowTransfer, ",")
	//routes = make(map[string]string)
	//if *routeList != "" {
	//	for _, s := range strings.Split(*routeList, ",") {
	//		s := strings.SplitN(s, "=", 2)
	//		if len(s) != 2 {
	//			log.Fatal("invalid -routes format")
	//		}
	//		if !strings.HasSuffix(s[0], ".") {
	//			s[0] += "."
	//		}
	//		routes[s[0]] = s[1]
	//	}
	//}

	udpServer := &dns.Server{Addr: *address, Net: "udp"}
	tcpServer := &dns.Server{Addr: *address, Net: "tcp"}
	dns.HandleFunc(".", route)
	go func() {
		if err := udpServer.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()
	go func() {
		if err := tcpServer.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait for SIGINT or SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	udpServer.Shutdown() // nolint: errcheck
	tcpServer.Shutdown() // nolint: errcheck
}

func route(w dns.ResponseWriter, req *dns.Msg) {
	//if len(req.Question) == 0 || !allowed(w, req) {
	//	dns.HandleFailed(w, req)
	//	return
	//}
	//for name, addr := range routes {
	//	if strings.HasSuffix(req.Question[0].Name, name) {
	//		proxy(addr, w, req)
	//		return
	//	}
	//}

	fallthroughs := make(map[int]struct{})
	for _, v := range strings.Split(*fallthroughStatuses, ",") {
		rcode, found := dns.StringToRcode[v]
		if !found {
			log.Fatalln("Could not find matching Rcode integer for", v)
		}

		fallthroughs[rcode] = struct{}{}
	}

	noFallthrough := strings.Split(*neverDefault, ",")

	proxy(*defaultServer, *prefixServer, *suffixServer, fallthroughs, noFallthrough, w, req)
}

//func isTransfer(req *dns.Msg) bool {
//	for _, q := range req.Question {
//		switch q.Qtype {
//		case dns.TypeIXFR, dns.TypeAXFR:
//			return true
//		}
//	}
//	return false
//}

//func allowed(w dns.ResponseWriter, req *dns.Msg) bool {
//	if !isTransfer(req) {
//		return true
//	}
//	remote, _, _ := net.SplitHostPort(w.RemoteAddr().String())
//	for _, ip := range transferIPs {
//		if ip == remote {
//			return true
//		}
//	}
//	return false
//}

func dnsRequestProxy(addr string, transport string, req *dns.Msg) (*dns.Msg, error) {
	c := &dns.Client{Net: transport}
	resp, _, err := c.Exchange(req, addr)
	return resp, err
}

func httpDNSRequestProxy(addr string, _ string, req *dns.Msg) (*dns.Msg, error) {
	httpreq, err := http.NewRequest(http.MethodGet, addr, nil)
	if err != nil {
		log.Errorln("Error setting up request:", err)
		return nil, err
	}

	qry := httpreq.URL.Query()
	qry.Add("name", req.Question[0].Name)
	qry.Add("type", fmt.Sprintf("%v", req.Question[0].Qtype))
	// qry.Add("cd", cdFlag) // Google DNS-over-HTTPS requires CD to be true - don't set it at all
	qry.Add("edns_client_subnet", "0.0.0.0/0")
	httpreq.URL.RawQuery = qry.Encode()

	if *debug {
		log.Debugln(httpreq.URL.String())
	}

	httpresp, err := http.DefaultClient.Do(httpreq)
	if err != nil {
		return nil, err
	}
	defer httpresp.Body.Close() // nolint: errcheck

	// Parse the JSON response
	dnsResp := new(DNSResponseJSON)
	decoder := json.NewDecoder(httpresp.Body)
	err = decoder.Decode(&dnsResp)
	if err != nil {
		return nil, err
	}

	// Parse the google Questions to DNS RRs
	questions := []dns.Question{}
	for idx, c := range dnsResp.Question {
		questions = append(questions, dns.Question{
			Name:   c.Name,
			Qtype:  uint16(c.Type),
			Qclass: req.Question[idx].Qclass,
		})
	}

	// Parse google RRs to DNS RRs
	answers := []dns.RR{}
	for _, a := range dnsResp.Answer {
		answers = append(answers, NewRR(a))
	}

	// Parse google RRs to DNS RRs
	authorities := []dns.RR{}
	for _, ns := range dnsResp.Authority {
		authorities = append(authorities, NewRR(ns))
	}

	// Parse google RRs to DNS RRs
	extras := []dns.RR{}
	for _, extra := range dnsResp.Additional {
		authorities = append(authorities, NewRR(extra))
	}

	resp := dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:                 req.Id,
			Response:           (dnsResp.Status == 0),
			Opcode:             dns.OpcodeQuery,
			Authoritative:      false,
			Truncated:          dnsResp.TC,
			RecursionDesired:   dnsResp.RD,
			RecursionAvailable: dnsResp.RA,
			//Zero: false,
			AuthenticatedData: dnsResp.AD,
			CheckingDisabled:  dnsResp.CD,
			Rcode:             int(dnsResp.Status),
		},
		Compress: req.Compress,
		Question: questions,
		Answer:   answers,
		Ns:       authorities,
		Extra:    extras,
	}

	return &resp, nil
}

func isSuccess(fallthroughStatuses map[int]struct{}, resp *dns.Msg) bool {
	if resp == nil {
		return false
	}
	_, found := fallthroughStatuses[resp.Rcode]
	return !found
}

func continueFallthrough(noFallthrough []string, req *dns.Msg) bool {
	for _, f := range noFallthrough {
		if f == "" {
			continue
		}
		for _, q := range req.Question {
			if strings.HasSuffix(q.Name, f) {
				return false
			}
		}
	}
	return true
}

type proxyFunc func() (*dns.Msg, error)

func proxy(addr string, prefixServer string, suffixServer string, fallthroughStatuses map[int]struct{},
	noFallthrough []string, w dns.ResponseWriter, req *dns.Msg) {

	qryCanFallthrough := continueFallthrough(noFallthrough, req)

	transport := "udp"
	if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
		transport = "tcp"
	}

	proxyFuncs := []proxyFunc{}

	// If prefix server set, try prefix server...
	if prefixServer != "" {
		proxyFuncs = append(proxyFuncs, func() (*dns.Msg, error) { return dnsRequestProxy(prefixServer, transport, req) })

	}

	proxyFuncs = append(proxyFuncs, func() (*dns.Msg, error) { return httpDNSRequestProxy(addr, transport, req) })

	// If prefix server set, try prefix server...
	if suffixServer != "" {
		proxyFuncs = append(proxyFuncs, func() (*dns.Msg, error) { return dnsRequestProxy(suffixServer, transport, req) })

	}

	for _, proxyFunc := range proxyFuncs {
		resp, err := proxyFunc()
		if err == nil && (isSuccess(fallthroughStatuses, resp) || !qryCanFallthrough) {
			// Write the response
			err = w.WriteMsg(resp)
			if err != nil {
				log.Errorln("Error writing DNS response:", err)
				dns.HandleFailed(w, req)
			}
			return
		}

		if !qryCanFallthrough {
			dns.HandleFailed(w, req)
			return
		}
	}

	dns.HandleFailed(w, req)
}
