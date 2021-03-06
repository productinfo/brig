package gateway

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"testing"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/sahib/brig/catfs"
	"github.com/sahib/brig/defaults"
	"github.com/sahib/config"
	"github.com/stretchr/testify/require"
)

func withBasicGateway(t *testing.T, fn func(gw *Gateway, fs *catfs.FS)) {
	os.RemoveAll("/tmp/test.db")

	cfg, err := config.Open(nil, defaults.Defaults, config.StrictnessPanic)
	require.Nil(t, err)

	fs, err := catfs.NewFilesystem(
		catfs.NewMemFsBackend(),
		"/tmp/test.db",
		"ali",
		false,
		cfg.Section("fs"),
	)

	require.Nil(t, err)

	cfg.SetBool("gateway.enabled", true)
	cfg.SetInt("gateway.port", 9999)
	gw := NewGateway(fs, cfg.Section("gateway"))
	gw.Start()

	defer func() {
		require.Nil(t, gw.Stop())
	}()

	time.Sleep(100 * time.Millisecond)
	fn(gw, fs)
}

func buildURL(gw *Gateway, suffix string) string {
	suffix = strings.TrimLeft(suffix, "/")
	return fmt.Sprintf("http://localhost:%d/%s", gw.cfg.Int("port"), suffix)
}

func ping(t *testing.T, gw *Gateway) bool {
	_, err := http.Get(buildURL(gw, ""))
	return err == nil
}

func query(t *testing.T, gw *Gateway, suffix string) (int, []byte) {
	resp, err := http.Get(buildURL(gw, suffix))
	require.Nil(t, err, fmt.Sprintf("%v", err))

	data, err := ioutil.ReadAll(resp.Body)
	require.Nil(t, err, fmt.Sprintf("%v", err))

	return resp.StatusCode, data
}

func queryWithAuth(t *testing.T, gw *Gateway, suffix, user, pass string) (int, []byte) {
	req, err := http.NewRequest("GET", buildURL(gw, suffix), nil)
	require.Nil(t, err, fmt.Sprintf("%v", err))

	req.SetBasicAuth(user, pass)
	client := &http.Client{}
	resp, err := client.Do(req)
	require.Nil(t, err, fmt.Sprintf("%v", err))

	data, err := ioutil.ReadAll(resp.Body)
	require.Nil(t, err, fmt.Sprintf("%v", err))

	return resp.StatusCode, data
}

func TestGatewayOK(t *testing.T) {
	withBasicGateway(t, func(gw *Gateway, fs *catfs.FS) {
		exampleData := []byte("Hello world")
		err := fs.Stage("/hello/world.png", bytes.NewReader(exampleData))
		require.Nil(t, err, fmt.Sprintf("%v", err))

		gw.cfg.SetStrings("folders", []string{"/"})
		status, data := query(t, gw, "/get/hello/world.png")
		require.Equal(t, 200, status)
		require.Equal(t, exampleData, data)
	})
}

func TestGatewayNoSuchFile(t *testing.T) {
	withBasicGateway(t, func(gw *Gateway, fs *catfs.FS) {
		gw.cfg.SetStrings("folders", []string{"/"})
		status, data := query(t, gw, "/get/hello/world.png")
		require.Equal(t, 404, status)
		require.Equal(t, []byte{}, data)
	})
}

func TestGatewayUnauthorizedBadFolder(t *testing.T) {
	withBasicGateway(t, func(gw *Gateway, fs *catfs.FS) {
		exampleData := []byte("Hello world")
		err := fs.Stage("/hello/world.png", bytes.NewReader(exampleData))
		require.Nil(t, err, fmt.Sprintf("%v", err))

		gw.cfg.SetStrings("folders", []string{"/world"})
		status, data := query(t, gw, "/get/hello/world.png")
		require.Equal(t, 401, status)
		require.Equal(t, []byte{}, data)
	})
}

func TestGatewayUnauthorizedBadUser(t *testing.T) {
	withBasicGateway(t, func(gw *Gateway, fs *catfs.FS) {
		exampleData := []byte("Hello world")
		err := fs.Stage("/hello/world.png", bytes.NewReader(exampleData))
		require.Nil(t, err, fmt.Sprintf("%v", err))

		gw.cfg.SetStrings("folders", []string{"/"})
		gw.cfg.SetBool("auth.enabled", true)
		gw.cfg.SetString("auth.user", "user")
		gw.cfg.SetString("auth.pass", "pass")

		status, data := queryWithAuth(t, gw, "/get/hello/world.png", "resu", "pass")
		require.Equal(t, 401, status)
		require.Equal(t, []byte{}, data)
	})
}

func TestGatewayUnauthorizedBadPass(t *testing.T) {
	withBasicGateway(t, func(gw *Gateway, fs *catfs.FS) {
		exampleData := []byte("Hello world")
		err := fs.Stage("/hello/world.png", bytes.NewReader(exampleData))
		require.Nil(t, err)

		gw.cfg.SetStrings("folders", []string{"/"})
		gw.cfg.SetBool("auth.enabled", true)
		gw.cfg.SetString("auth.user", "user")
		gw.cfg.SetString("auth.pass", "pass")

		status, data := queryWithAuth(t, gw, "/get/hello/world.png", "user", "ssap")
		require.Equal(t, 401, status)
		require.Equal(t, []byte{}, data)
	})
}

func TestGatewayConfigChangeEnabled(t *testing.T) {
	withBasicGateway(t, func(gw *Gateway, fs *catfs.FS) {
		exampleData := []byte("Hello world")
		err := fs.Stage("/hello/world.png", bytes.NewReader(exampleData))
		require.Nil(t, err)

		gw.cfg.SetStrings("folders", []string{"/"})
		require.True(t, ping(t, gw))
		status, data := query(t, gw, "/get/hello/world.png")
		require.Equal(t, 200, status)
		require.Equal(t, exampleData, data)

		gw.cfg.SetBool("enabled", false)
		time.Sleep(10 * time.Millisecond)

		require.False(t, ping(t, gw))
	})
}

func TestGatewayConfigChangePort(t *testing.T) {
	withBasicGateway(t, func(gw *Gateway, fs *catfs.FS) {
		exampleData := []byte("Hello world")
		err := fs.Stage("/hello/world.png", bytes.NewReader(exampleData))
		require.Nil(t, err)

		gw.cfg.SetStrings("folders", []string{"/"})
		require.True(t, ping(t, gw))
		status, data := query(t, gw, "/get/hello/world.png")
		require.Equal(t, 200, status)
		require.Equal(t, exampleData, data)

		gw.cfg.SetInt("port", 5001)
		time.Sleep(1 * time.Second)

		// should still work, the port changed.
		status, data = query(t, gw, "/get/hello/world.png")
		require.Equal(t, 200, status)
		require.Equal(t, exampleData, data)
	})
}

func TestBlock(t *testing.T) {
	t.Skip("This test is only for debugging purposes")

	log.SetLevel(log.DebugLevel)
	withBasicGateway(t, func(gw *Gateway, fs *catfs.FS) {
		exampleData := []byte("Hello world")
		err := fs.Stage("/hello/world.png", bytes.NewReader(exampleData))
		require.Nil(t, err)

		gw.cfg.SetStrings("folders", []string{"/"})
		gw.cfg.SetString("cert.domain", "nwzmlh4iouqikobq.myfritz.net")
		gw.cfg.SetString("cert.certfile", "/tmp/fullchain.pem")
		gw.cfg.SetString("cert.keyfile", "/tmp/privkey.pem")

		ch := make(chan os.Signal)
		signal.Notify(ch, os.Interrupt)
		<-ch
	})
}

// TODO: Test for directory.
// TODO: Tests for upcoming cert stuff.
