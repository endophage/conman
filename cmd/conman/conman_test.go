package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateApplicationSanityCheckSuccess(t *testing.T) {
	fakeImageFile := []byte("this is an image, not really")
	http.HandleFunc("/spotify.png", func(w http.ResponseWriter, r *http.Request) {
		w.Write(fakeImageFile)
	})
	server := httptest.NewServer(http.DefaultServeMux)
	defer server.Close()

	shasum := sha256.Sum256(fakeImageFile)
	shasum64 := base64.StdEncoding.EncodeToString(shasum[:])

	customInfo := []byte(fmt.Sprintf(`{
		"desktop": "[Desktop Entry]\nType=Application\nName=Spotify\nIcon=spotify\nExec=\"docker run --rm -v /etc/localtime:/etc/localtime:ro -v $HOME/.spotify:/home/spotify -v /tmp/.X11-unix:/tmp/.X11-unix -v /run/user/$UID/pulse/native:/pulse -e DISPLAY=unix$DISPLAY --device /dev/snd:/dev/snd --name spotify spotify:latest\"\nTerminal=false",
		"icon": {
			"url": "%s/spotify.png",
			"checksum": {
				"sha256": "%s"
			},
			"size": %v
		}
	}`, server.URL, shasum64, len(fakeImageFile)))

	fakeHomeDir, err := ioutil.TempDir("", "homedir")
	require.NoError(t, err)
	defer os.RemoveAll(fakeHomeDir)

	require.NoError(t, CreateApplication(fakeHomeDir, "spotify", customInfo))

	_, err = os.Stat(filepath.Join(fakeHomeDir, IconDirRelHome, "spotify.png"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(fakeHomeDir, AppDirRelHome, "spotify.desktop"))
	require.NoError(t, err)
}
