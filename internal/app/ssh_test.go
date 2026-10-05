package app

import (
	"bytes"
	"context"
	"errors"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"net"
	"testing"
	"time"
)

func TestSSHHostPinAndPublicKeyAuthentication(t *testing.T) {
	f := fixture(t)
	host, e := ssh.ParsePrivateKey(f.HostKey)
	if e != nil {
		t.Fatal(e)
	}
	public, _, _, _, _ := ssh.ParseAuthorizedKey(f.SSHPublic)
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if bytes.Equal(key.Marshal(), public.Marshal()) {
			return nil, nil
		}
		return nil, errors.New("not authorized")
	}}
	cfg.AddHostKey(host)
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			raw, e := ln.Accept()
			if e != nil {
				return
			}
			go func() {
				raw.SetDeadline(time.Now().Add(15 * time.Second))
				defer raw.Close()
				conn, chans, requests, e := ssh.NewServerConn(raw, cfg)
				if e != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(requests)
				for newChannel := range chans {
					if newChannel.ChannelType() != "session" {
						newChannel.Reject(ssh.UnknownChannelType, "only sessions supported")
						continue
					}
					channel, reqs, e := newChannel.Accept()
					if e != nil {
						return
					}
					go func() {
						defer channel.Close()
						for req := range reqs {
							if req.Type == "subsystem" {
								var sub struct{ Name string }
								ssh.Unmarshal(req.Payload, &sub)
								if sub.Name == "sftp" {
									req.Reply(true, nil)
									server := sftp.NewRequestServer(channel, sftp.InMemHandler())
									server.Serve()
									server.Close()
									return
								}
							}
							req.Reply(false, nil)
						}
					}()
				}
			}()
		}
	}()
	defer func() { ln.Close(); <-done }()
	target := Target{Server: ServerConfig{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, User: "certree", HostKey: string(f.HostPublic), Type: "nginx"}, SSHKey: f.SSHKey}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, e := connect(ctx, target)
	if e != nil {
		t.Fatal("valid pinned connection failed", e)
	}
	r.Close()
	target.Server.HostKey = string(f.SSHPublic)
	if r, e := connect(ctx, target); e == nil {
		r.Close()
		t.Fatal("changed host key accepted")
	}
	target.Server.HostKey = string(f.HostPublic)
	target.SSHKey = f.HostKey
	if r, e := connect(ctx, target); e == nil {
		r.Close()
		t.Fatal("unregistered client key accepted")
	}
}
