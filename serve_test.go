package tailscale

import (
	"context"
	"net"
	"reflect"
	"sort"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"

	clog "github.com/coredns/coredns/plugin/pkg/log"
)

func newTS() Tailscale {
	return Tailscale{
		zone: "example.com",
		entries: map[string]map[string][]string{
			"test1": {
				"A":    []string{"127.0.0.1"},
				"AAAA": []string{"::1"},
			},
			"test2-1": {
				"A":    []string{"127.0.0.1"},
				"AAAA": []string{"::1"},
			},
			"test2-2": {
				"A":    []string{"127.0.0.1"},
				"AAAA": []string{"::1"},
			},
			"test2": {
				"CNAME": []string{"test2-1.example.com", "test2-2.example.com"},
			},
		},
	}
}

func TestServeDNSFallback(t *testing.T) {
	clog.D.Set()
	ts := newTS()
	ts.fall.SetZonesFromArgs(nil)

	test3 := net.ParseIP("100.100.100.100")

	// No match, no next plugin.
	var msg dns.Msg
	msg.SetQuestion("test3.example.com", dns.TypeA)
	resp, err := ts.ServeDNS(context.Background(), dnstest.NewRecorder(&test.ResponseWriter{}), &msg)
	if err == nil {
		t.Fatal("expected error, got none")
	}
	if want, got := dns.RcodeServerFailure, resp; got != want {
		t.Fatalf("want response code %d, got %d", want, got)
	}

	ts.next = plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		msg := dns.Msg{}
		msg.SetReply(r)
		msg.Answer = append(msg.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: "test3.example.com", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
			A:   test3,
		})
		if err := w.WriteMsg(&msg); err != nil {
			return dns.RcodeServerFailure, err
		}
		return dns.RcodeSuccess, nil
	})

	// Match, next plugin configured.
	msg.SetQuestion("test1.example.com", dns.TypeA)
	w := dnstest.NewRecorder(&test.ResponseWriter{})
	resp, err = ts.ServeDNS(context.Background(), w, &msg)
	if want, got := dns.RcodeSuccess, resp; got != want {
		t.Fatalf("want response code %d, got %d", want, got)
	}
	if want, got := net.ParseIP("127.0.0.1"), w.Msg.Answer[0].(*dns.A).A; !got.Equal(want) {
		t.Errorf("want %s, got: %s", want, got)
	}

	// No match, next plugin configured.
	msg.SetQuestion("test3.example.com", dns.TypeA)
	w = dnstest.NewRecorder(&test.ResponseWriter{})
	ts.ServeDNS(context.Background(), w, &msg)

	if w.Msg == nil {
		t.Fatal("no answer")
	}
	if want, got := 1, len(w.Msg.Answer); want != got {
		t.Fatalf("want %d answer, got: %d", want, got)
	}
	if got := w.Msg.Answer[0].(*dns.A).A; !got.Equal(test3) {
		t.Errorf("want %s, got: %s", test3, got)
	}
}

func TestServeDNSNoFallback(t *testing.T) {
	clog.D.Set()
	ts := newTS()

	// No match
	var msg dns.Msg
	msg.SetQuestion("test3.example.com", dns.TypeA)
	resp, err := ts.ServeDNS(context.Background(), dnstest.NewRecorder(&test.ResponseWriter{}), &msg)
	if err != nil {
		t.Fatal("unexpected error")
	}
	if want, got := dns.RcodeNameError, resp; got != want {
		t.Fatalf("want response code %d, got %d", want, got)
	}

	// Match
	msg.SetQuestion("test1.example.com", dns.TypeA)
	w := dnstest.NewRecorder(&test.ResponseWriter{})
	resp, err = ts.ServeDNS(context.Background(), w, &msg)
	if want, got := dns.RcodeSuccess, resp; got != want {
		t.Fatalf("want response code %d, got %d", want, got)
	}
	if want, got := net.ParseIP("127.0.0.1"), w.Msg.Answer[0].(*dns.A).A; !got.Equal(want) {
		t.Errorf("want %s, got: %s", want, got)
	}

}

func TestResolveA(t *testing.T) {
	clog.D.Set()
	ts := newTS()
	msg := dns.Msg{}

	domain := "test1.example.com"

	ts.resolveA(domain, &msg)

	testEquals(t, "answer count", 1, len(msg.Answer))
	testEquals(t, "query name", domain, msg.Answer[0].Header().Name)

	if a, ok := msg.Answer[0].(*dns.A); ok {
		testEquals(t, "A record", "127.0.0.1", a.A.String())
	} else {
		t.Errorf("Expected AAAA return RR value type")
	}

}

func TestResolveAAAA(t *testing.T) {
	clog.D.Set()
	ts := newTS()
	msg := dns.Msg{}

	domain := "test1.example.com"

	ts.resolveAAAA(domain, &msg)

	testEquals(t, "answer count", 1, len(msg.Answer))
	testEquals(t, "query name", domain, msg.Answer[0].Header().Name)

	if a, ok := msg.Answer[0].(*dns.AAAA); ok {
		testEquals(t, "A record", "::1", a.AAAA.String())
	} else {
		t.Errorf("Expected AAAA return RR value")
	}

}

func TestResolveCNAME(t *testing.T) {
	clog.D.Set()
	ts := newTS()
	msg := dns.Msg{}
	domain := "test2.example.com"

	ts.resolveCNAME(domain, &msg, TypeAll)

	testEquals(t, "answer count", 6, len(msg.Answer))

	var cnames []string
	var as []string
	var aaaas []string
	for _, rr := range msg.Answer {

		switch rec := rr.(type) {

		case *dns.CNAME:
			cnames = append(cnames, rec.Target)

		case *dns.A:
			as = append(as, rec.A.String())

		case *dns.AAAA:
			aaaas = append(aaaas, rec.AAAA.String())
		}

	}

	sort.Strings(cnames)
	sort.Strings(as)
	testEquals(t, "CNAME record", []string{"test2-1.example.com", "test2-2.example.com"}, cnames)
	testEquals(t, "A record", []string{"127.0.0.1", "127.0.0.1"}, as)
	testEquals(t, "AAAA record", []string{"::1", "::1"}, aaaas)

}

func TestResolveAIsCNAME(t *testing.T) {
	clog.D.Set()
	ts := newTS()
	msg := dns.Msg{}
	domain := "test2.example.com"

	ts.resolveA(domain, &msg)

	testEquals(t, "answer count", 4, len(msg.Answer))

	var cnames []string
	var as []string
	for _, rr := range msg.Answer {

		switch rec := rr.(type) {

		case *dns.CNAME:
			cnames = append(cnames, rec.Target)

		case *dns.A:
			as = append(as, rec.A.String())

		}

	}

	sort.Strings(cnames)
	sort.Strings(as)
	testEquals(t, "CNAME record", []string{"test2-1.example.com", "test2-2.example.com"}, cnames)
	testEquals(t, "A record", []string{"127.0.0.1", "127.0.0.1"}, as)
}

func TestResolveAAAAIsCNAME(t *testing.T) {
	clog.D.Set()
	ts := newTS()
	msg := dns.Msg{}
	domain := "test2.example.com"

	ts.resolveAAAA(domain, &msg)

	testEquals(t, "answer count", 4, len(msg.Answer))

	var cnames []string
	var aaaas []string
	for _, rr := range msg.Answer {

		switch rec := rr.(type) {

		case *dns.CNAME:
			cnames = append(cnames, rec.Target)

		case *dns.AAAA:
			aaaas = append(aaaas, rec.AAAA.String())

		}

	}

	sort.Strings(cnames)
	sort.Strings(aaaas)
	testEquals(t, "CNAME record", []string{"test2-1.example.com", "test2-2.example.com"}, cnames)
	testEquals(t, "AAAA record", []string{"::1", "::1"}, aaaas)
}

func TestSubdomainResolution(t *testing.T) {
	clog.D.Set()
	ts := newTS()

	testCases := []struct {
		name      string
		query     string
		qtype     uint16
		expectIP  string
		rcode     int
		checkFunc func(t *testing.T, msg *dns.Msg)
	}{
		{
			name:     "simple subdomain A record",
			query:    "sub.test1.example.com",
			qtype:    dns.TypeA,
			expectIP: "127.0.0.1",
			rcode:    dns.RcodeSuccess,
		},
		{
			name:     "deep subdomain A record",
			query:    "a.b.c.d.test1.example.com",
			qtype:    dns.TypeA,
			expectIP: "127.0.0.1",
			rcode:    dns.RcodeSuccess,
		},
		{
			name:     "subdomain AAAA record",
			query:    "sub.test1.example.com",
			qtype:    dns.TypeAAAA,
			expectIP: "::1",
			rcode:    dns.RcodeSuccess,
		},
		{
			name:     "deep subdomain AAAA record",
			query:    "deep.sub.test1.example.com",
			qtype:    dns.TypeAAAA,
			expectIP: "::1",
			rcode:    dns.RcodeSuccess,
		},
		{
			name:  "subdomain of nonexistent host",
			query: "sub.nonexistent.example.com",
			qtype: dns.TypeA,
			rcode: dns.RcodeNameError,
		},
		{
			name:  "subdomain of existing host CNAME",
			query: "something.test2.example.com",
			qtype: dns.TypeA,
			rcode: dns.RcodeSuccess,
			checkFunc: func(t *testing.T, msg *dns.Msg) {
				var hasCNAME, hasA bool
				for _, rr := range msg.Answer {
					switch rr.(type) {
					case *dns.CNAME:
						hasCNAME = true
					case *dns.A:
						hasA = true
					}
				}
				if !hasCNAME {
					t.Error("Expected CNAME records in response")
				}
				if !hasA {
					t.Error("Expected A records in response")
				}
			},
		},
		{
			name:  "CNAME request on subdomain",
			query: "sub.sub.test2.example.com",
			qtype: dns.TypeCNAME,
			rcode: dns.RcodeSuccess,
			checkFunc: func(t *testing.T, msg *dns.Msg) {
				var hasCNAME bool
				for _, rr := range msg.Answer {
					if _, ok := rr.(*dns.CNAME); ok {
						hasCNAME = true
						break
					}
				}
				if !hasCNAME {
					t.Error("Expected CNAME records in response")
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var msg dns.Msg
			msg.SetQuestion(tc.query, tc.qtype)
			w := dnstest.NewRecorder(&test.ResponseWriter{})
			resp, err := ts.ServeDNS(context.Background(), w, &msg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if want, got := tc.rcode, resp; got != want {
				t.Fatalf("want response code %d, got %d", want, got)
			}

			if tc.expectIP != "" && len(w.Msg.Answer) > 0 {
				var gotIP net.IP

				switch tc.qtype {
				case dns.TypeA:
					if a, ok := w.Msg.Answer[0].(*dns.A); ok {
						gotIP = a.A
					} else {
						for _, ans := range w.Msg.Answer {
							if a, ok := ans.(*dns.A); ok {
								gotIP = a.A
								break
							}
						}
					}
				case dns.TypeAAAA:
					if aaaa, ok := w.Msg.Answer[0].(*dns.AAAA); ok {
						gotIP = aaaa.AAAA
					} else {
						for _, ans := range w.Msg.Answer {
							if aaaa, ok := ans.(*dns.AAAA); ok {
								gotIP = aaaa.AAAA
								break
							}
						}
					}
				}

				if gotIP != nil {
					if want, got := net.ParseIP(tc.expectIP), gotIP; !got.Equal(want) {
						t.Errorf("want %s, got: %s", want, got)
					}
				} else if tc.expectIP != "" {
					t.Errorf("expected IP %s but no matching record found", tc.expectIP)
				}
			}

			if tc.checkFunc != nil {
				tc.checkFunc(t, w.Msg)
			}
		})
	}
}

func testEquals(t *testing.T, msg string, expected interface{}, received interface{}) {
	if !reflect.DeepEqual(expected, received) {
		t.Errorf("Expected %s %s: received %s", msg, expected, received)
	}

}
