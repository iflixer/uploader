// s3probe is a standalone diagnostic that reproduces "NoSuchUpload" without any
// application code, to produce evidence for the S3 provider's support.
//
// It keeps several multipart uploads open, adds a 5 MiB part to each every -interval,
// checks each session with ListParts, and logs every HTTP request/response
// (remote IP, status, x-amz-request-id, x-amz-id-2, Server, Date). When a session
// disappears it logs the exact error response and which sibling sessions still exist.
//
// Usage (same env as the uploader):
//
//	S3_ENDPOINT=tpfx.s3nomad.com S3_REGION=eu-central-1 S3_BUCKET=tpfx-mnt01 \
//	S3_KEY_ID=... S3_SECRET=... go run ./cmd/s3probe -sessions 3 -interval 20s -duration 60m
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const partSize = 5 * 1024 * 1024 // minimal multipart part size

var (
	logMu   sync.Mutex
	logSink io.Writer = os.Stdout
)

func logf(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Fprintf(logSink, "%s %s\n", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), fmt.Sprintf(format, args...))
}

// tracingRT logs every HTTP round trip made by minio-go, including its internal retries.
type tracingRT struct{ base http.RoundTripper }

func (t *tracingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	var remote string
	var reused bool
	trace := &httptrace.ClientTrace{
		GotConn: func(i httptrace.GotConnInfo) {
			remote = i.Conn.RemoteAddr().String()
			reused = i.Reused
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	dur := time.Since(start).Round(time.Millisecond)
	q := req.URL.RawQuery
	if len(q) > 80 {
		q = q[:80] + "..."
	}
	if err != nil {
		logf("HTTP %s %s?%s remote=%s reused=%v dur=%s NETERR=%v", req.Method, req.URL.Path, q, remote, reused, dur, err)
		return resp, err
	}
	logf("HTTP %s %s?%s remote=%s reused=%v dur=%s status=%d req-id=%q id-2=%q server=%q date=%q",
		req.Method, req.URL.Path, q, remote, reused, dur, resp.StatusCode,
		resp.Header.Get("x-amz-request-id"), resp.Header.Get("x-amz-id-2"),
		resp.Header.Get("Server"), resp.Header.Get("Date"))
	return resp, nil
}

type lossEvent struct {
	at        time.Time
	session   int
	uploadID  string
	age       time.Duration
	parts     int
	errDetail string
}

type prober struct {
	core     *minio.Core
	cli      *minio.Client
	bucket   string
	prefix   string
	interval time.Duration
	buf      []byte

	mu     sync.Mutex
	events []lossEvent
}

func errDetail(err error) string {
	er := minio.ToErrorResponse(err)
	if er.Code == "" {
		return err.Error()
	}
	return fmt.Sprintf("code=%s message=%q status=%d requestID=%s hostID=%s resource=%s server=%s",
		er.Code, er.Message, er.StatusCode, er.RequestID, er.HostID, er.Resource, er.Server)
}

func isLost(err error) bool {
	return minio.ToErrorResponse(err).Code == "NoSuchUpload"
}

func (p *prober) newSession(ctx context.Context, n int) (key, uploadID string, err error) {
	key = fmt.Sprintf("%ss%d-%d", p.prefix, n, time.Now().UnixNano())
	uploadID, err = p.core.NewMultipartUpload(ctx, p.bucket, key, minio.PutObjectOptions{})
	return
}

func (p *prober) worker(ctx context.Context, n int, wg *sync.WaitGroup) {
	defer wg.Done()
	for ctx.Err() == nil {
		key, uploadID, err := p.newSession(ctx, n)
		if err != nil {
			logf("S%d CREATE_FAILED %s", n, errDetail(err))
			sleep(ctx, 10*time.Second)
			continue
		}
		created := time.Now()
		logf("S%d CREATED key=%s uploadID=%s", n, key, uploadID)

		parts := 0
		lost := false
		for !lost && ctx.Err() == nil {
			parts++
			_, err := p.core.PutObjectPart(ctx, p.bucket, key, uploadID, parts, bytes.NewReader(p.buf), int64(len(p.buf)), minio.PutObjectPartOptions{})
			if err == nil {
				_, err = p.core.ListObjectParts(ctx, p.bucket, key, uploadID, 0, 1000)
			}
			switch {
			case err == nil:
				logf("S%d OK part=%d age=%s", n, parts, time.Since(created).Round(time.Second))
			case ctx.Err() != nil:
			case isLost(err):
				lost = true
				ev := lossEvent{time.Now(), n, uploadID, time.Since(created), parts, errDetail(err)}
				p.mu.Lock()
				p.events = append(p.events, ev)
				p.mu.Unlock()
				logf("S%d *** SESSION_LOST *** age=%s parts_sent=%d uploadID=%s %s", n, ev.age.Round(time.Second), parts, uploadID, ev.errDetail)
				p.listSurvivors(ctx)
			default:
				parts--
				logf("S%d ERROR (not NoSuchUpload) %s", n, errDetail(err))
			}
			if !lost {
				sleep(ctx, p.interval)
			}
		}
		if !lost && ctx.Err() != nil {
			actx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			_ = p.core.AbortMultipartUpload(actx, p.bucket, key, uploadID)
			cancel()
		}
	}
}

// listSurvivors shows which probe sessions the server still knows about right after a loss.
func (p *prober) listSurvivors(ctx context.Context) {
	res, err := p.core.ListMultipartUploads(ctx, p.bucket, p.prefix, "", "", "", 1000)
	if err != nil {
		logf("LIST_UPLOADS failed: %s", errDetail(err))
		return
	}
	var ids []string
	for _, u := range res.Uploads {
		ids = append(ids, u.UploadID)
	}
	logf("LIST_UPLOADS server still has %d probe session(s): %v", len(ids), ids)
}

// heartbeat proves the service itself stays reachable while sessions vanish.
func (p *prober) heartbeat(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for ctx.Err() == nil {
		start := time.Now()
		_, err := p.cli.BucketExists(ctx, p.bucket)
		if err != nil && ctx.Err() == nil {
			logf("HEARTBEAT FAIL dur=%s %s", time.Since(start).Round(time.Millisecond), errDetail(err))
		} else if err == nil {
			logf("HEARTBEAT ok dur=%s", time.Since(start).Round(time.Millisecond))
		}
		sleep(ctx, 10*time.Second)
	}
}

// dnsWatch logs the endpoint's resolved addresses; several IPs or changes hint at a multi-node pool.
func dnsWatch(ctx context.Context, host string, wg *sync.WaitGroup) {
	defer wg.Done()
	last := ""
	for ctx.Err() == nil {
		addrs, err := net.DefaultResolver.LookupHost(ctx, host)
		sort.Strings(addrs)
		cur := strings.Join(addrs, ",")
		if err != nil && ctx.Err() == nil {
			logf("DNS %s lookup error: %v", host, err)
		} else if cur != last && err == nil {
			logf("DNS %s -> %s", host, cur)
			last = cur
		}
		sleep(ctx, 30*time.Second)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func (p *prober) summary() {
	p.mu.Lock()
	defer p.mu.Unlock()
	logf("================ SUMMARY ================")
	if len(p.events) == 0 {
		logf("No sessions were lost during the run.")
		return
	}
	sort.Slice(p.events, func(i, j int) bool { return p.events[i].at.Before(p.events[j].at) })
	for _, e := range p.events {
		logf("LOST session=%d at=%s age=%s parts=%d uploadID=%s", e.session, e.at.Format("15:04:05.000"), e.age.Round(time.Second), e.parts, e.uploadID)
	}
	// Group losses within 15s of each other: a cluster of different sessions
	// dying together points to a server-side event, not a client problem.
	i := 0
	for i < len(p.events) {
		j := i + 1
		for j < len(p.events) && p.events[j].at.Sub(p.events[j-1].at) < 15*time.Second {
			j++
		}
		logf("EVENT %s UTC: %d session(s) lost within %s", p.events[i].at.Format("15:04:05"), j-i, p.events[j-1].at.Sub(p.events[i].at).Round(time.Second))
		i = j
	}
}

func env(name string) string {
	v := os.Getenv(name)
	if v == "" {
		fmt.Fprintf(os.Stderr, "env %s is required\n", name)
		os.Exit(2)
	}
	return v
}

func main() {
	sessions := flag.Int("sessions", 3, "concurrent multipart sessions")
	interval := flag.Duration("interval", 20*time.Second, "pause between parts of one session")
	duration := flag.Duration("duration", 60*time.Minute, "total run time")
	logFile := flag.String("log", fmt.Sprintf("s3probe-%s.log", time.Now().UTC().Format("20060102-150405")), "log file (also printed to stdout)")
	flag.Parse()

	f, err := os.Create(*logFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	logSink = io.MultiWriter(os.Stdout, f)

	endpoint := strings.TrimPrefix(strings.TrimPrefix(env("S3_ENDPOINT"), "https://"), "http://")
	endpoint = strings.TrimSuffix(endpoint, "/")
	bucket := env("S3_BUCKET")

	base := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     120 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  true,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
	}
	opts := &minio.Options{
		Creds:     credentials.NewStaticV4(env("S3_KEY_ID"), env("S3_SECRET"), ""),
		Secure:    true,
		Region:    env("S3_REGION"),
		Transport: &tracingRT{base: base},
	}
	cli, err := minio.New(endpoint, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	core := &minio.Core{Client: cli}

	buf := make([]byte, partSize)
	if _, err := rand.Read(buf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	runID := time.Now().UTC().Format("20060102-150405")
	p := &prober{core: core, cli: cli, bucket: bucket, prefix: "probe/" + runID + "/", interval: *interval, buf: buf}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *duration)
	defer cancel()

	logf("START endpoint=%s bucket=%s sessions=%d interval=%s duration=%s prefix=%s", endpoint, bucket, *sessions, *interval, *duration, p.prefix)

	var wg sync.WaitGroup
	wg.Add(2)
	go dnsWatch(ctx, endpoint, &wg)
	go p.heartbeat(ctx, &wg)
	for n := 1; n <= *sessions; n++ {
		wg.Add(1)
		go p.worker(ctx, n, &wg)
	}
	wg.Wait()

	// Clean up leftovers of this run.
	cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ccancel()
	for obj := range cli.ListIncompleteUploads(cctx, bucket, p.prefix, true) {
		if obj.Err == nil {
			err := core.AbortMultipartUpload(cctx, bucket, obj.Key, obj.UploadID)
			if err != nil && !errors.Is(err, context.Canceled) && !isLost(err) {
				logf("cleanup abort %s: %s", obj.Key, errDetail(err))
			}
		}
	}
	p.summary()
	logf("DONE. Send %s to support.", *logFile)
}
