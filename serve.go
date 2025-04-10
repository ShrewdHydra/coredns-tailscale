package tailscale

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
)

var log = clog.NewWithPlugin("tailscale")

const (
	TypeAll = iota
	TypeA
	TypeAAAA
)

// ServeDNS implements the plugin.Handler interface. This method gets called when tailscale is used
// in a Server.

func (t *Tailscale) resolveA(domainName string, msg *dns.Msg) {
	log.Debugf("Resolving A record for %s in zone %s", domainName, t.zone)

	// Get the number of labels in common between domain and zone
	numCommonLabels := dns.CompareDomainName(domainName, t.zone)
	labels := dns.SplitDomainName(domainName)
	log.Debugf("Domain has %d labels, zone has %d labels in common", len(labels), numCommonLabels)

	name := labels[len(labels)-numCommonLabels-1]
	log.Debugf("Extracted base name: %s", name)

	// Look for an A record
	entries, ok := t.entries[name]["A"]
	if ok {
		log.Debugf("Found A record for %s with %d entries", name, len(entries))
	} else {
		log.Debugf("No A record found for %s", name)
	}

	if ok {
		log.Debugf("Adding A records for %s to response", name)
		for _, entry := range entries {
			log.Debugf("  - Adding A record: %s", entry)
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: domainName, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(entry),
			})
		}
	} else {
		// There's no A record, so see if a CNAME exists
		log.Debug("No v4 entry after lookup, so trying CNAME")
		t.resolveCNAME(domainName, msg, TypeA)
	}
}

func (t *Tailscale) resolveAAAA(domainName string, msg *dns.Msg) {
	log.Debugf("Resolving AAAA record for %s in zone %s", domainName, t.zone)

	// Get the number of labels in common between domain and zone
	numCommonLabels := dns.CompareDomainName(domainName, t.zone)
	labels := dns.SplitDomainName(domainName)
	log.Debugf("Domain has %d labels, zone has %d labels in common", len(labels), numCommonLabels)

	name := labels[len(labels)-numCommonLabels-1]
	log.Debugf("Extracted base name: %s", name)

	// Look for an AAAA record
	entries, ok := t.entries[name]["AAAA"]
	if ok {
		log.Debugf("Found AAAA record for %s with %d entries", name, len(entries))
	} else {
		log.Debugf("No AAAA record found for %s", name)
	}

	if ok {
		log.Debugf("Adding AAAA records for %s to response", name)
		for _, entry := range entries {
			log.Debugf("  - Adding AAAA record: %s", entry)
			msg.Answer = append(msg.Answer, &dns.AAAA{
				Hdr:  dns.RR_Header{Name: domainName, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60},
				AAAA: net.ParseIP(entry),
			})
		}
	} else {
		// There's no AAAA record, so see if a CNAME exists
		log.Debug("No v6 entry after lookup, so trying CNAME")
		t.resolveCNAME(domainName, msg, TypeAAAA)
	}
}

func (t *Tailscale) resolveCNAME(domainName string, msg *dns.Msg, lookupType int) {
	log.Debugf("Resolving CNAME record for %s in zone %s", domainName, t.zone)

	// Get the number of labels in common between domain and zone
	numCommonLabels := dns.CompareDomainName(domainName, t.zone)
	labels := dns.SplitDomainName(domainName)
	log.Debugf("Domain has %d labels, zone has %d labels in common", len(labels), numCommonLabels)

	indexUniqueLabels := len(labels) - numCommonLabels - 1
	name := labels[indexUniqueLabels]
	log.Debugf("Extracted base name: %s", name)

	// Look for a CNAME record
	targets, ok := t.entries[name]["CNAME"]
	if ok {
		log.Debugf("Found CNAME record for %s with %d entries", name, len(targets))
	} else {
		log.Debugf("No CNAME record found for %s", name)
	}

	if ok {
		log.Debugf("Adding CNAME records for %s to response", name)
		for _, target := range targets {
			targetDomain := target
			if indexUniqueLabels > 0 {
				targetDomain = strings.Join(labels[:indexUniqueLabels], ".") + "." + target
			}
			log.Debugf("  - Adding CNAME record: %s", targetDomain)
			msg.Answer = append(msg.Answer, &dns.CNAME{
				Hdr:    dns.RR_Header{Name: domainName, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60},
				Target: targetDomain,
			})

			// Resolve local zone A or AAAA records if they exist for the referenced target
			if lookupType == TypeAll || lookupType == TypeA {
				log.Debug("CNAME record found, lookup up local recursive A")
				t.resolveA(targetDomain, msg)
			}
			if lookupType == TypeAll || lookupType == TypeAAAA {
				log.Debug("CNAME record found, lookup up local recursive AAAA")
				t.resolveAAAA(targetDomain, msg)
			}
		}
	}
}

func (t *Tailscale) handleNoRecords(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, msg *dns.Msg) (int, error) {
	log.Debugf("No records found for %s, checking fallthrough", r.Question[0].Name)
	if t.fall.Through(r.Question[0].Name) {
		log.Debug("falling through to next plugin")
		return plugin.NextOrFailure(t.Name(), t.next, ctx, w, r)
	} else {
		log.Debug("No records and no fallthrough, returning NXDOMAIN")
		RcodeCount.WithLabelValues(dns.RcodeToString[dns.RcodeNameError], metrics.WithServer(ctx)).Inc()
		if err := w.WriteMsg(msg); err != nil {
			log.Warningf("Error writing NXDOMAIN response: %v", err)
			return dns.RcodeServerFailure, err
		}
		return dns.RcodeNameError, nil
	}
}

func (t *Tailscale) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()
	queryType := dns.TypeToString[r.Question[0].Qtype]
	log.Debugf("Handling Tailscale %s query for %s", queryType, qname)

	// Check if the query is for a zone we're authoritative for
	if !dns.IsSubDomain(t.zone, qname) || qname == t.zone {
		log.Debug("Domain is not in zone, returning")
		return plugin.NextOrFailure(t.Name(), t.next, ctx, w, r)
	}

	RequestCount.WithLabelValues(metrics.WithServer(ctx), queryType).Inc()

	start := time.Now()
	log.Debugf("Tailscale peers list has %d entries", len(t.entries))
	log.Debugf("Configured zone: %s", t.zone)

	// if len(t.entries) > 0 {
	// 	log.Debug("Available entries:")
	// 	for name, types := range t.entries {
	// 		log.Debugf("  ├─ Host: %s", name)
	// 		for recordType, values := range types {
	// 			log.Debugf("  │  ├─ %s: %v", recordType, values)
	// 		}
	// 	}
	// }

	msg := dns.Msg{}
	msg.SetReply(r)
	msg.Authoritative = true

	t.mu.RLock()
	defer t.mu.RUnlock()

	switch r.Question[0].Qtype {
	case dns.TypeA:
		t.resolveA(qname, &msg)

	case dns.TypeAAAA:
		t.resolveAAAA(qname, &msg)

	case dns.TypeCNAME:
		t.resolveCNAME(qname, &msg, TypeAll)
	}

	if len(msg.Answer) > 0 {
		log.Debugf("Sending response with %d answers", len(msg.Answer))
		RcodeCount.WithLabelValues(dns.RcodeToString[dns.RcodeSuccess], metrics.WithServer(ctx)).Inc()
		if err := w.WriteMsg(&msg); err != nil {
			log.Warningf("Error writing response: %v", err)
			return dns.RcodeServerFailure, err
		}
		RequestDuration.WithLabelValues(metrics.WithServer(ctx)).Observe(time.Since(start).Seconds())
		return dns.RcodeSuccess, nil
	} else {
		log.Debug("No answers in response")
		code, err := t.handleNoRecords(ctx, w, r, &msg)
		RequestDuration.WithLabelValues(metrics.WithServer(ctx)).Observe(time.Since(start).Seconds())
		return code, err
	}
}
