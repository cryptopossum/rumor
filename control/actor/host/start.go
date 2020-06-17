package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	mplex "github.com/libp2p/go-libp2p-mplex"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	secio "github.com/libp2p/go-libp2p-secio"
	yamux "github.com/libp2p/go-libp2p-yamux"
	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/addrutil"
	"strings"
	"time"
)

type HostStartCmd struct {
	*base.Base
	WithSetHost
	PrivKeyStr       string        `ask:"--priv" help:"hex-encoded RSA private key for libp2p host. Random if none is specified."`
	TransportsStrArr []string      `ask:"--transport" help:"Transports to use. Options: tcp, ws"`
	MuxStrArr        []string      `ask:"--mux" help:"Multiplexers to use"`
	SecurityStr      string        `ask:"--security" help:"Security to use. Options: secio, none"`
	RelayEnabled     bool          `ask:"--relay" help:"enable relayer functionality"`
	LoPeers          int           `ask:"--lo-peers" help:"low-water for connection manager to trim peer count to"`
	HiPeers          int           `ask:"--hi-peers" help:"high-water for connection manager to trim peer count from"`
	GracePeriod      time.Duration `ask:"--peer-grace-period" help:"Time to grace a peer from being trimmed"`
	NatEnabled       bool          `ask:"--nat" help:"enable nat address discovery (upnp/pmp)"`
}

func (c *HostStartCmd) Default() {
	c.TransportsStrArr = []string{"tcp"}
	c.MuxStrArr = []string{"yamux", "mplex"}
	c.SecurityStr = "secio"
	c.LoPeers = 15
	c.HiPeers = 20
	c.GracePeriod = 20 * time.Second
	c.NatEnabled = true
}

func (c *HostStartCmd) Help() string {
	return "Start the host node. See flags for security, transport, mux etc. options"
}

func (c *HostStartCmd) Run(ctx context.Context, args ...string) error {
	_, err := c.Host()
	if err == nil {
		return errors.New("already have a host open")
	}
	var priv crypto.PrivKey
	{
		if c.PrivKeyStr == "" { // generate new private key if non was specified
			var err error
			priv, _, err = crypto.GenerateKeyPairWithReader(crypto.Secp256k1, -1, rand.Reader)
			if err != nil {
				return err
			}
			p, err := priv.Raw()
			if err != nil {
				return err
			}
			c.Log.WithField("priv", hex.EncodeToString(p)).Info("Generated random Secp256k1 private key")
		} else {
			p, err := addrutil.ParsePrivateKey(c.PrivKeyStr)
			if err != nil {
				return err
			}
			priv = (*crypto.Secp256k1PrivateKey)(p)
		}
	}
	hostOptions := make([]libp2p.Option, 0)

	for _, v := range c.TransportsStrArr {
		v = strings.ToLower(strings.TrimSpace(v))
		switch v {
		case "tcp":
			hostOptions = append(hostOptions, libp2p.Transport(tcp.NewTCPTransport))
		case "ws":
			hostOptions = append(hostOptions, libp2p.Transport(ws.New))
		default:
			return fmt.Errorf("could not recognize transport %s", v)
		}
	}

	for _, v := range c.MuxStrArr {
		v = strings.ToLower(strings.TrimSpace(v))
		switch v {
		case "yamux":
			hostOptions = append(hostOptions, libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport))
		case "mplex":
			hostOptions = append(hostOptions, libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport))
		default:
			return fmt.Errorf("could not recognize mux %s", v)
		}
	}

	{
		switch c.SecurityStr {
		case "none":
			// no security, for debugging etc.
		case "secio":
			hostOptions = append(hostOptions, libp2p.Security(secio.ID, secio.New))
		default:
			return fmt.Errorf("could not recognize security %s", c.SecurityStr)
		}
	}

	if c.NatEnabled {
		hostOptions = append(hostOptions, libp2p.NATPortMap())
	}

	if c.RelayEnabled {
		hostOptions = append(hostOptions, libp2p.EnableRelay())
	}

	hostOptions = append(hostOptions,
		libp2p.Identity(priv),
		libp2p.Peerstore(pstoremem.NewPeerstore()), // TODO: persist peerstore?
		libp2p.ConnectionManager(connmgr.NewConnManager(c.LoPeers, c.HiPeers, c.GracePeriod)),
	)
	// Not the command ctx, we want the host to stay open after the command.
	h, err := libp2p.New(c.BaseContext, hostOptions...)
	if err != nil {
		return err
	}
	return c.SetHost(h)
}