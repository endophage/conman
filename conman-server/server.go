package main

import (
	"net"
	"net/http"
	"time"

	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution/registry/client/auth"
	"github.com/docker/distribution/registry/client/transport"
	"github.com/docker/docker/pkg/term"
	"github.com/riyazdf/notary/client"
	"github.com/riyazdf/notary/tuf/data"
	"net/url"
	"os"
	"strings"
)

var tr http.RoundTripper

// Hardcode notary server to the docker hub
const server = "https://notary.docker.io"

// Hardcode trust dir to .docker/trust
const trustDir = ".docker/trust"

func init() {
	tr = getTransport("docker.io/conman/apps", true)
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	http.HandleFunc("/", listTagHandler)
	http.ListenAndServe(":8080", nil)
}

type Custom struct {
	Cmd       string   `json:"cmd,omitempty"`
	Desktop   string   `json:"desktop"`
	Icon      IconMeta `json:"icon"`
	Mimetypes []string `json:"mimetypes"`
}

type IconMeta struct {
	Url      string      `json:"url"`
	Checksum data.Hashes `json:"checksum"`
}

func listTagHandler(w http.ResponseWriter, r *http.Request) {
	notaryRepo, err := client.NewNotaryRepository(trustDir, "docker.io/conman/apps", server, tr, nil)
	if err != nil {
		logrus.Error("Could not retrieve notary repo", err)
		w.WriteHeader(500)
		return
	}

	// List all targets
	targets, err := notaryRepo.ListTargets()
	if err != nil {
		logrus.Error("Could not list targets", err)
		w.WriteHeader(500)
		return
	}

	// Set the access control header
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	targetInfo := make([]map[string]string, 0, len(targets))
	logrus.Debugf("Retrieved %d targets", len(targets))
	for _, t := range targets {
		logrus.Debugf("Trying to unmarshal custom field: %s", t.Custom)
		var customInfo = Custom{}
		customBytes := t.Custom[1 : len(t.Custom)-1]
		customBytes, err = base64.StdEncoding.DecodeString(string(customBytes))
		if err != nil {
			logrus.Error("error decoding custom field: ", err)
			w.WriteHeader(500)
			return
		}
		// This is so janky
		customBytes = customBytes[1 : len(customBytes)-1]
		logrus.Debugf("Trying to decode custom field AGAIN: %s", customBytes)
		customBytes, err = base64.StdEncoding.DecodeString(string(customBytes))
		if err != nil {
			logrus.Error("error decoding custom field: ", err)
			w.WriteHeader(500)
			return
		}
		logrus.Debugf("After base64 decoding custom field: %s", string(customBytes))
		err = json.Unmarshal(customBytes, &customInfo)
		if err != nil {
			logrus.Error("error unmarshalling custom field: ", err)
			w.WriteHeader(500)
			return
		}
		targetInfo = append(targetInfo, map[string]string{"Name": t.Name, "URL": customInfo.Icon.Url})
	}

	targetBytes, err := json.Marshal(targetInfo)
	if err != nil {
		logrus.Error("error during json marshal", err)
		w.WriteHeader(500)
		return
	}
	w.Write(targetBytes)
}

// getTransport returns an http.RoundTripper to be used for all http requests.
// It correctly handles the auth challenge/credentials required to interact
// with a notary server over both HTTP Basic Auth and the JWT auth implemented
// in the notary-server
// The readOnly flag indicates if the operation should be performed as an
// anonymous read only operation. If the command entered requires write
// permissions on the server, readOnly must be false
func getTransport(gun string, readOnly bool) http.RoundTripper {

	base := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}
	return tokenAuth("https://notary.docker.io", base, gun, readOnly)
}

type passwordStore struct {
	anonymous bool
}

func (ps passwordStore) Basic(u *url.URL) (string, string) {
	if ps.anonymous {
		return "", ""
	}

	stdin := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stdout, "Enter username: ")

	userIn, err := stdin.ReadBytes('\n')
	if err != nil {
		logrus.Errorf("error processing username input: %s", err)
		return "", ""
	}

	username := strings.TrimSpace(string(userIn))

	state, err := term.SaveState(0)
	if err != nil {
		logrus.Errorf("error saving terminal state, cannot retrieve password: %s", err)
		return "", ""
	}
	term.DisableEcho(0, state)
	defer term.RestoreTerminal(0, state)

	fmt.Fprintf(os.Stdout, "Enter password: ")

	userIn, err = stdin.ReadBytes('\n')
	fmt.Fprintln(os.Stdout)
	if err != nil {
		logrus.Errorf("error processing password input: %s", err)
		return "", ""
	}
	password := strings.TrimSpace(string(userIn))

	return username, password
}

func tokenAuth(trustServerURL string, baseTransport *http.Transport, gun string,
	readOnly bool) http.RoundTripper {

	// TODO(dmcgowan): add notary specific headers
	authTransport := transport.NewTransport(baseTransport)
	pingClient := &http.Client{
		Transport: authTransport,
		Timeout:   5 * time.Second,
	}
	endpoint, err := url.Parse(trustServerURL)
	if err != nil {
		logrus.Fatalf("Could not parse remote trust server url (%s): %s", trustServerURL, err.Error())
	}
	if endpoint.Scheme == "" {
		logrus.Fatalf("Trust server url has to be in the form of http(s)://URL:PORT. Got: %s", trustServerURL)
	}
	subPath, err := url.Parse("v2/")
	if err != nil {
		logrus.Fatalf("Failed to parse v2: %s", err.Error())
	}
	endpoint = endpoint.ResolveReference(subPath)
	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		logrus.Fatal(err)
	}
	resp, err := pingClient.Do(req)
	if err != nil {
		logrus.Errorf("could not reach %s: %s", trustServerURL, err.Error())
		logrus.Fatal(err)
	}
	// non-nil err means we must close body
	defer resp.Body.Close()
	if (resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices) &&
		resp.StatusCode != http.StatusUnauthorized {
		// If we didn't get a 2XX range or 401 status code, we're not talking to a notary server.
		// The http client should be configured to handle redirects so at this point, 3XX is
		// not a valid status code.
		logrus.Errorf("could not reach %s: %d", trustServerURL, resp.StatusCode)
		logrus.Fatal(err)
	}

	challengeManager := auth.NewSimpleChallengeManager()
	if err := challengeManager.AddResponse(resp); err != nil {
		logrus.Fatal(err)
	}

	ps := passwordStore{anonymous: readOnly}

	var actions []string
	if readOnly {
		actions = []string{"pull"}
	} else {
		actions = []string{"push", "pull"}
	}
	tokenHandler := auth.NewTokenHandler(authTransport, ps, gun, actions...)
	basicHandler := auth.NewBasicHandler(ps)
	modifier := transport.RequestModifier(auth.NewAuthorizer(challengeManager, tokenHandler, basicHandler))
	return transport.NewTransport(baseTransport, modifier)
}
