package net

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"zombiezen.com/go/capnproto2/rpc"

	log "github.com/Sirupsen/logrus"
	"github.com/sahib/brig/backend"
	"github.com/sahib/brig/net/capnp"
	"github.com/sahib/brig/net/peer"
	"github.com/sahib/brig/repo"
	"github.com/sahib/brig/util/server"
)

type Server struct {
	bk         backend.Backend
	baseServer *server.Server
	hdl        *connHandler
	pingMap    *PingMap
}

func (sv *Server) Serve() error {
	return sv.baseServer.Serve()
}

func (sv *Server) Close() error {
	return sv.baseServer.Close()
}

func (sv *Server) Quit() {
	sv.baseServer.Quit()
}

func publishSelf(bk backend.Backend, owner string) error {
	// Example: alice@wonderland.org/resource
	name := peer.Name(owner)

	// Publish the full name »alice@wonderland.org/resource«
	if err := bk.PublishName(owner); err != nil {
		return err
	}

	// Also publish »alice@wonderland.org«
	if noRes := name.WithoutResource(); noRes != string(name) {
		if err := bk.PublishName(noRes); err != nil {
			return err
		}
	}

	// Publish »wonderland.org«
	if domain := name.Domain(); domain != "" {
		if err := bk.PublishName(domain); err != nil {
			return err
		}
	}

	// Publish »alice«
	if user := name.User(); user != string(name) {
		if err := bk.PublishName(user); err != nil {
			return err
		}
	}

	return nil
}

func NewServer(rp *repo.Repository, bk backend.Backend) (*Server, error) {
	hdl := &connHandler{
		rp: rp,
		bk: bk,
	}

	log.Infof("creating new listener")
	lst, err := bk.Listen("brig/caprpc")
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	log.Infof("creating new server")
	baseServer, err := server.NewServer(lst, hdl, ctx)
	if err != nil {
		return nil, err
	}

	log.Infof("publish self")
	if err := publishSelf(bk, rp.Owner); err != nil {
		log.Warningf("Failed to publish `%v` to the network: %v", rp.Owner, err)
		log.Warningf("You will not be visible to other users.")
	}

	return &Server{
		baseServer: baseServer,
		bk:         bk,
		hdl:        hdl,
		pingMap:    NewPingMap(bk),
	}, nil
}

const (
	LocateNone  = 0
	LocateExact = 1 << iota
	LocateDomain
	LocateUser
	LocateEmail
	LocateAll = LocateExact | LocateDomain | LocateUser | LocateEmail
)

type LocateMask int

func (lm LocateMask) String() string {
	if lm == LocateNone {
		return ""
	}

	parts := []string{}
	if lm&LocateExact != 0 {
		parts = append(parts, "exact")
	}
	if lm&LocateDomain != 0 {
		parts = append(parts, "domain")
	}
	if lm&LocateUser != 0 {
		parts = append(parts, "user")
	}
	if lm&LocateEmail != 0 {
		parts = append(parts, "email")
	}

	return strings.Join(parts, ",")
}

func LocateMaskFromString(s string) (LocateMask, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return LocateNone, nil
	}

	mask := LocateMask(LocateNone)
	parts := strings.Split(s, ",")
	for _, part := range parts {
		switch part {
		case "exact":
			mask |= LocateExact
		case "domain":
			mask |= LocateDomain
		case "user":
			mask |= LocateUser
		case "email":
			mask |= LocateEmail
		case "all":
			mask |= LocateAll
		default:
			return mask, fmt.Errorf("Invalid locate mask name `%s`", part)
		}
	}

	return mask, nil
}

// LocateResult is one result returned by Locate's result channel.
type LocateResult struct {
	Peers []peer.Info
	Mask  LocateMask
	Name  string
	Err   error
}

func (sv *Server) Locate(ctx context.Context, who peer.Name, mask LocateMask) chan LocateResult {
	uniqueNames := make(map[string]LocateMask)

	// Example: donald@whitehouse.gov/ovaloffice
	uniqueNames[string(who)] = mask & LocateExact

	// Example: whitehouse.gov
	uniqueNames[who.Domain()] = mask & LocateDomain

	// Example: donald
	uniqueNames[who.User()] = mask & LocateUser

	// Example: donald@whitehouse.gov
	uniqueNames[who.WithoutResource()] = mask & LocateEmail

	resultCh := make(chan LocateResult)

	wg := &sync.WaitGroup{}
	for name, mask := range uniqueNames {
		if name == "" {
			continue
		}

		// It's not enabled:
		if mask == 0 {
			continue
		}

		wg.Add(1)

		go func(name string, mask LocateMask) {
			defer wg.Done()

			peers, err := sv.bk.ResolveName(ctx, name)
			log.Debugf("Found peers: %v", peers)
			resultCh <- LocateResult{
				Peers: peers,
				Err:   err,
				Name:  name,
				Mask:  mask,
			}
		}(name, mask)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	return resultCh
}

// PeekFingerprint fetches the fingerprint of a peer without authenticating
// ourselves or them.
func (sv *Server) PeekFingerprint(ctx context.Context, addr string) (peer.Fingerprint, string, error) {
	// Query the remotes pubkey and use it to build the remotes' fingerprint.
	// If not available we just send an empty string back to the client.
	pubKey, remoteName, err := PeekRemotePubkey(addr, sv.hdl.rp, sv.bk, ctx)
	if err != nil {
		log.Warningf(
			"locate: failed to dial to `%s` (%s): %v",
			addr, addr, err,
		)
		return peer.Fingerprint(""), "", nil
	}

	log.Debugf("remote name: %v", remoteName)
	return peer.BuildFingerprint(addr, pubKey), remoteName, nil
}

func (sv *Server) Identity() (peer.Info, error) {
	return sv.bk.Identity()
}

func (sv *Server) PingMap() *PingMap {
	return sv.pingMap
}

func (sv *Server) IsOnline() bool {
	return sv.bk.IsOnline()
}

func (sv *Server) Connect() error {
	return sv.bk.Connect()
}

func (sv *Server) Disconnect() error {
	return sv.bk.Disconnect()
}

/////////////////////////////////////
// INTERNAL HANDLER IMPLEMENTATION //
/////////////////////////////////////

type connHandler struct {
	bk backend.Backend
	rp *repo.Repository
}

// Handle is called whenever we receive a new connection from another brig peer.
func (hdl *connHandler) Handle(ctx context.Context, conn net.Conn) {
	// We are currently not allowing more than one parallel connection.
	// This is not a technical problem, but more due to the fact that it makes
	// it easier to pass the current remote to the active handler.
	// Make sure to reset the current remote:
	keyring := hdl.rp.Keyring()
	ownPubKey, err := keyring.OwnPubKey()
	if err != nil {
		log.Warnf("Failed to retrieve own pubkey: %v", err)
		return
	}

	// The respective handler should get its own context it can listen to.
	reqCtx, reqCancel := context.WithCancel(ctx)
	reqHdl := &requestHandler{
		bk:  hdl.bk,
		rp:  hdl.rp,
		ctx: reqCtx,
	}

	// This func will be called during the authentication process.
	// It checks if the pub key the other side send us can be
	// related to one of the allowed remotes. If not, the connection
	// will be dropped.
	authChecker := func(pubKey []byte) error {
		remotes, err := hdl.rp.Remotes.ListRemotes()
		if err != nil {
			return err
		}

		// Create a temporary fingerprint to get a hashed version of pubkey.
		remoteFp := peer.BuildFingerprint("", pubKey)

		// Linear scan over all remotes.
		// If this proves to be a performance problem, we can fix it later.
		for _, remote := range remotes {
			if remote.Fingerprint.PubKeyID() == remoteFp.PubKeyID() {
				log.Infof("starting connection with addr `%s`", remote.Fingerprint.Addr())
				reqHdl.currRemoteName = remote.Name
				return nil
			}
		}

		return fmt.Errorf("remote uses no public key known to us")
	}

	// Take the raw connection we get and add an authentication layer on top of it.
	authConn := NewAuthReadWriter(conn, keyring, ownPubKey, hdl.rp.Owner, authChecker)

	// Trigger the authentication. This is not strictly necessary and would
	// happen anyways on the first read/write on the connection. But doing it
	// here catches errors early.
	if err := authConn.Trigger(); err != nil {
		log.Warnf("failed to authenticate connection: %v", err)
		return
	}

	// The connection is considered authenticated at this point.
	// Initialize the capnp rpc protocol over it.
	transport := rpc.StreamTransport(conn)
	srv := capnp.API_ServerToClient(reqHdl)
	rpcConn := rpc.NewConn(transport, rpc.MainInterface(srv.Client))

	// Wait until either side quits the connection in the background.
	// The number of open connections is limited by the base server.
	go func() {
		defer reqCancel()

		if err := rpcConn.Wait(); err != nil {
			log.Warnf("serving rpc failed: %v", err)
		}

		if err := rpcConn.Close(); err != nil {
			// Close seems to be complaining that the conn was
			// already closed, but be safe and expect this.
			if err != rpc.ErrConnClosed {
				log.Warnf("failed to close rpc conn: %v", err)
			}
		}
	}()
}

// Quit is being called by the base server implementation
func (hdl *connHandler) Quit() error {
	return nil
}
