package datatug

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// WatchCoordination identifies the project's explicitly selected DataTug daemon.
// Storage settings, user tokens, runner credentials and personal definitions
// belong to the coordinator deployment, never this shared project descriptor.
type WatchCoordination struct {
	Endpoint  string `json:"endpoint" firestore:"endpoint"`
	Namespace string `json:"namespace" firestore:"namespace"`
}

var coordinationNamespace = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func (v *WatchCoordination) Validate() error {
	if v == nil {
		return nil
	}
	u, err := url.Parse(v.Endpoint)
	if err != nil || u.Host == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.TrimSpace(v.Endpoint) != v.Endpoint || !coordinationNamespace.MatchString(v.Namespace) {
		return errors.New("coordination requires a credential-free absolute endpoint and namespace")
	}
	loopback := strings.EqualFold(u.Hostname(), "localhost")
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return errors.New("coordination endpoint requires HTTPS, or HTTP on loopback")
	}
	return nil
}

func (v *WatchCoordination) UnmarshalJSON(body []byte) error {
	type descriptor WatchCoordination
	var decoded descriptor
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return errors.New("coordination must be an object")
	}
	if err := d.Decode(&decoded); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("invalid coordination descriptor")
	}
	value := WatchCoordination(decoded)
	if err := value.Validate(); err != nil {
		return err
	}
	*v = value
	return nil
}
