//This file is modified version from https://github.com/shadowsocks/go-shadowsocks2

package main

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/shadowsocks/go-shadowsocks2/socks"
)

// Create a SOCKS server listening on addr and proxy to server.
func socksLocal(addr, server string, shadow func(net.Conn) net.Conn) {
	logger.Info("SOCKS proxy %s <-> %s", addr, server)
	tcpLocal(addr, server, shadow, func(c net.Conn) (socks.Addr, error) { return socks.Handshake(c) })
}

// Create a TCP tunnel from addr to target via server.
func tcpTun(addr, server, target string, shadow func(net.Conn) net.Conn) {
	tgt := socks.ParseAddr(target)
	if tgt == nil {
		logger.Warning("invalid target address %q", target)
		return
	}
	logger.Info("TCP tunnel %s <-> %s <-> %s", addr, server, target)
	tcpLocal(addr, server, shadow, func(net.Conn) (socks.Addr, error) { return tgt, nil })
}

// Listen on addr and proxy to server to reach target from getAddr.
func tcpLocal(addr, server string, shadow func(net.Conn) net.Conn, getAddr func(net.Conn) (socks.Addr, error)) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Warning("failed to listen on %s: %v", addr, err)
		return
	}

	for {
		c, err := l.Accept()
		if err != nil {
			logger.Warning("failed to accept: %s", err)
			continue
		}

		go func() {
			defer c.Close()
			c.(*net.TCPConn).SetKeepAlive(true)
			tgt, err := getAddr(c)
			if err != nil {

				// UDP: keep the connection until disconnect then free the UDP socket
				if err == socks.InfoUDPAssociate {
					buf := []byte{}
					// block here
					for {
						_, err := c.Read(buf)
						if err, ok := err.(net.Error); ok && err.Timeout() {
							continue
						}
						logger.Info("UDP Associate End.")
						return
					}
				}

				logger.Warning("failed to get target address: %v", err)
				return
			}

			rc, err := net.Dial("tcp", server)
			if err != nil {
				logger.Warning("failed to connect to server %v: %v", server, err)
				return
			}
			defer rc.Close()
			rc.(*net.TCPConn).SetKeepAlive(true)
			rc = shadow(rc)

			if _, err = rc.Write(tgt); err != nil {
				logger.Warning("failed to send target address: %v", err)
				return
			}

			logger.Info("proxy %s <-> %s <-> %s", c.RemoteAddr(), server, tgt)
			_, _, err = relay(rc, c)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				logger.Warning("relay error: %v", err)
			}
		}()
	}
}

// Listen on addr for incoming connections.
func tcpRemote(host string, port int, shadow func(net.Conn) net.Conn) {
	addr := fmt.Sprintf("%s:%d", host, port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Warning("failed to listen on %s: %v", addr, err)
		return
	}

	logger.Info("listening TCP on %s", addr)
	for {
		c, err := l.Accept()
		if err != nil {
			logger.Warning("failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()
			c.(*net.TCPConn).SetKeepAlive(true)
			c = shadow(c)

			tgt, err := socks.ReadAddr(c)
			if err != nil {
				logger.Warning("failed to get target address: %v", err)
				return
			}

			rc, err := net.Dial("tcp", tgt.String())
			if err != nil {
				logger.Warning("failed to connect to target: %v", err)
				return
			}
			defer rc.Close()
			rc.(*net.TCPConn).SetKeepAlive(true)

			logger.Info("proxy %s <-> %s", c.RemoteAddr(), tgt)
			wLen, rLen, err := relay(c, rc)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				logger.Warning("relay error: %v", err)
			}
			if infuxdb != nil {
				infuxdb.writeDataMeta(&DataMeta{
					Host:      host,
					Port:      port,
					CAddr:     c.RemoteAddr().String(),
					SAddr:     tgt.String(),
					RLen:      rLen,
					WLen:      wLen,
					Timestamp: time.Now(),
				})
			}
		}()
	}
}

// relay copies between left and right bidirectionally. Returns number of
// bytes copied from right to left, from left to right, and any error occurred.
func relay(left, right net.Conn) (int64, int64, error) {
	type res struct {
		N   int64
		Err error
	}
	ch := make(chan res)

	go func() {
		n, err := io.Copy(right, left)
		right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
		left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
		ch <- res{n, err}
	}()

	n, err := io.Copy(left, right)
	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
	rs := <-ch

	if err == nil {
		err = rs.Err
	}
	return n, rs.N, err
}
