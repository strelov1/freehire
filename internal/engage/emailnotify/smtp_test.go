package emailnotify

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

func TestParseSMTPURL(t *testing.T) {
	r, err := parseSMTPURL("smtps://resend:re_key@smtp.resend.com")
	if err != nil || r.host != "smtp.resend.com" || r.port != "465" {
		t.Fatalf("got %+v, %v", r, err)
	}
	for _, bad := range []string{"smtp://u:p@h:587", "smtps://h:465", "smtps://u@h:465", "smtps://u:p@:465"} {
		if _, err := parseSMTPURL(bad); err == nil {
			t.Errorf("%s: want error", bad)
		}
	}
}

// fakeSMTP answers one SMTP session over TLS and returns what DATA carried.
func fakeSMTP(t *testing.T) (*smtpRelay, <-chan string) {
	t.Helper()
	srv := httptest.NewUnstartedServer(nil)
	srv.StartTLS() // only for its certificate and trusting client config
	cert := srv.TLS.Certificates[0]
	pool := srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	srv.Close()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		rd := bufio.NewReader(c)
		say := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		say("220 fake")
		var data strings.Builder
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				say("250-fake")
				say("250 AUTH PLAIN")
			case strings.HasPrefix(cmd, "AUTH"):
				say("235 ok")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				say("250 ok")
			case cmd == "DATA":
				say("354 go")
				for {
					l, _ := rd.ReadString('\n')
					if l == ".\r\n" {
						break
					}
					data.WriteString(l)
				}
				got <- data.String()
				say("250 queued")
			case cmd == "QUIT":
				say("221 bye")
				return
			}
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	r, err := parseSMTPURL("smtps://user:pass@127.0.0.1:" + port)
	if err != nil {
		t.Fatal(err)
	}
	r.tls.RootCAs = pool
	r.tls.ServerName = "example.com" // httptest's certificate names example.com
	return r, got
}

func TestClient_SendGoesThroughTheSMTPRelay(t *testing.T) {
	relay, got := fakeSMTP(t)
	f := &fakeSES{}
	c := &Client{ses: f, relay: relay}
	err := c.Send(context.Background(), Message{
		From: "freehire <notifications@mail.freehire.me>", To: "a@example.com",
		Subject: "Hi", HTML: "<p>x</p>", Text: "x",
		Group: emailprefs.GroupAlerts, UnsubscribeURL: "https://freehire.me/u/1",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := <-got
	for _, want := range []string{"To: a@example.com", "List-Unsubscribe: <https://freehire.me/u/1>"} {
		if !strings.Contains(body, want) {
			t.Errorf("message lacks %q", want)
		}
	}
	if f.in != nil {
		t.Error("SES was called with a relay set")
	}
	if err := c.WithEngagementTracking("set").CheckEngagementTracking(context.Background()); err != nil {
		t.Errorf("tracking check with a relay: %v", err)
	}
}

func TestClient_AnUnreachableRelayTripsTheBreaker(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = ln.Close() // nothing listens: every dial is refused
	relay, err := parseSMTPURL("smtps://user:pass@127.0.0.1:" + port)
	if err != nil {
		t.Fatal(err)
	}
	c := (&Client{ses: &fakeSES{}, relay: relay}).WithSendingBreaker()
	m := Message{From: "n@mail.freehire.me", To: "a@example.com", Subject: "s", HTML: "h", Text: "t", Group: emailprefs.GroupEssential}
	for i := 0; i < 2; i++ {
		if err := c.Send(context.Background(), m); !errors.Is(err, ErrSendingPaused) {
			t.Fatalf("send %d: got %v, want ErrSendingPaused", i, err)
		}
	}
}
