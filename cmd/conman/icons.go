package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/riyazdf/notary/tuf/data"
)

// AllowedIconTypes are the allowed extensions for the icon
var AllowedIconTypes = []string{"png"}

// IconInfo contains information about the Icon for a ConMan app
type IconInfo struct {
	Filename string
	URL      string
	Checksum data.Hashes
	Size     int64
	Type     string
}

// UnmarshalJSON checks that it's a valid IconInfo object
func (iconInfo *IconInfo) UnmarshalJSON(input []byte) error {
	i := &struct {
		URL      string      `json:"url"`
		Checksum data.Hashes `json:"checksum"`
		Size     int64       `json:"size"`
	}{}
	if err := json.Unmarshal(input, i); err != nil {
		return err
	}

	if len(i.Checksum) == 0 {
		return fmt.Errorf("invalid icon checksum information")
	}

	urlParsed, err := url.Parse(i.URL)
	if err != nil {
		return err
	}

	info := &IconInfo{
		URL:      urlParsed.String(),
		Checksum: i.Checksum,
		Size:     i.Size,
	}
	extension := strings.TrimPrefix(strings.ToLower(path.Ext(i.URL)), ".")

	for _, ext := range AllowedIconTypes {
		if extension == ext {
			info.Type = ext
			*iconInfo = *info
			return nil
		}
	}
	return fmt.Errorf("invalid icon type: %s", extension)
}

// Download downloads the icon bytes as specified by IconInfo
func (iconInfo *IconInfo) Download() ([]byte, error) {
	req, err := http.NewRequest("GET", iconInfo.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not download icon at %s", iconInfo.URL)
	}

	// if resp.ContentLength > iconInfo.Size {
	// 	return nil, fmt.Errorf("icon size too big")
	// }

	// // b := io.LimitReader(resp.Body, iconInfo.Size)
	// body, err := ioutil.ReadAll(b)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := data.CheckHashes(body, iconInfo.Checksum); err != nil {
		return nil, err
	}

	return body, nil
}
