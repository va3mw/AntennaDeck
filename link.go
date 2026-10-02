package main

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var errNotConnected = errors.New("not connected")

// link keeps a raw TCP socket to one device open, reconnecting as needed.
// Each device supplies callbacks for connect, incoming bytes and a periodic tick.
type link struct {
	name         string
	app          *App
	addr         func() (string, bool) // address, enabled
	interval     func() time.Duration  // tick interval
	staleAfter   time.Duration         // reconnect if nothing received for this long (0 = never)
	onConnect    func()
	onDisconnect func()
	onData       func([]byte)
	onTick       func()

	restartCh chan struct{}
	mu        sync.Mutex
	conn      net.Conn
	lastRx    atomic.Int64
	quiet     atomic.Bool // device connected but silent; suppress repeated log lines
	lastErr   string
}

func newLink(name string, app *App) *link {
	return &link{name: name, app: app, restartCh: make(chan struct{}, 1)}
}

func hostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// restart drops the current connection so new settings take effect.
func (l *link) restart() {
	select {
	case l.restartCh <- struct{}{}:
	default:
	}
}

func (l *link) send(b []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return errNotConnected
	}
	_ = l.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err := l.conn.Write(b)
	return err
}

func (l *link) sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-l.restartCh:
		return true
	case <-time.After(d):
		return true
	}
}

func (l *link) logErr(msg string) {
	if msg != l.lastErr {
		l.lastErr = msg
		l.app.logf("%s: %s", l.name, msg)
	}
}

func (l *link) run(ctx context.Context) {
	for ctx.Err() == nil {
		addr, enabled := l.addr()
		if !enabled {
			l.lastErr = ""
			if !l.sleep(ctx, 2*time.Second) {
				return
			}
			continue
		}

		c, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			l.logErr("cannot connect to " + addr + " (" + err.Error() + ")")
			if !l.sleep(ctx, 3*time.Second) {
				return
			}
			continue
		}
		if tc, ok := c.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(10 * time.Second)
		}
		l.lastErr = ""
		if !l.quiet.Load() {
			l.app.logf("%s: connected to %s", l.name, addr)
		}
		l.mu.Lock()
		l.conn = c
		l.mu.Unlock()
		l.lastRx.Store(time.Now().UnixNano())
		select { // discard a stale restart request
		case <-l.restartCh:
		default:
		}
		if l.onConnect != nil {
			l.onConnect()
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 4096)
			for {
				n, err := c.Read(buf)
				if n > 0 {
					l.lastRx.Store(time.Now().UnixNano())
					if l.quiet.Swap(false) {
						l.app.logf("%s: device is responding", l.name)
					}
					l.onData(append([]byte(nil), buf[:n]...))
				}
				if err != nil {
					return
				}
			}
		}()

		iv := l.interval()
		if iv < 100*time.Millisecond {
			iv = time.Second
		}
		ticker := time.NewTicker(iv)
		reason := "connection closed"
	loop:
		for {
			select {
			case <-ctx.Done():
				reason = ""
				break loop
			case <-l.restartCh:
				reason = "settings changed, reconnecting"
				break loop
			case <-done:
				break loop
			case <-ticker.C:
				if l.staleAfter > 0 && time.Since(time.Unix(0, l.lastRx.Load())) > l.staleAfter {
					reason = "no response from device, reconnecting"
					if l.quiet.Swap(true) {
						reason = "" // already reported
					}
					break loop
				}
				if l.onTick != nil {
					l.onTick()
				}
			}
		}
		ticker.Stop()
		l.mu.Lock()
		l.conn = nil
		l.mu.Unlock()
		_ = c.Close()
		<-done
		if l.onDisconnect != nil {
			l.onDisconnect()
		}
		if reason != "" {
			l.app.logf("%s: %s", l.name, reason)
		}
		if !l.sleep(ctx, time.Second) {
			return
		}
	}
}
