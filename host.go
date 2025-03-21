package herd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/spf13/cast"
	"golang.org/x/crypto/ssh"
)

// Hosts can have attributes of any types, but querying is limited to strings,
// booleans, numbers, nil and slices of these values.
type HostAttributes map[string]interface{}

func (h HostAttributes) prefix(prefix string) HostAttributes {
	ret := make(map[string]interface{})
	for k, v := range h {
		ret[prefix+k] = v
	}
	return ret
}

type Connection interface {
	Close()
}

// A host represents a remote host. It can be instantiated manually, but is
// usually fetched from one or more Providers, which can all contribute to the
// hosts attributes.
type Host struct {
	Name       string
	Address    string
	Attributes HostAttributes
	Connection io.Closer
	publicKeys []ssh.PublicKey
	lastResult *Result
	csum       uint32
}

type host Host

func (h *Host) UnmarshalJSON(data []byte) error {
	var h2 host
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(&h2); err != nil {
		return err
	}
	for k, v := range h2.Attributes {
		if n, ok := v.(json.Number); ok {
			if i, err := n.Int64(); err == nil {
				h2.Attributes[k] = i
			} else {
				h2.Attributes[k], _ = n.Float64()
			}
		}
	}
	*h = Host(h2)
	h.init()
	return nil
}

// Hosts should be initialized with this function, which also initializes any
// internal data, without which SSH connections will not be possible.
func NewHost(name, address string, attributes HostAttributes) *Host {
	h := &Host{
		Name:       name,
		Address:    address,
		Attributes: attributes,
	}
	h.init()
	return h
}

// Set all the defults and initialize ssh configuration for the host
func (h *Host) init() {
	h.csum = crc32.ChecksumIEEE([]byte(h.Name))
	h.publicKeys = make([]ssh.PublicKey, 0)

	if h.Attributes == nil {
		h.Attributes = make(HostAttributes)
	}
	parts := strings.SplitN(h.Name, ".", 2)
	h.Attributes["hostname"] = parts[0]
	if len(parts) == 2 {
		h.Attributes["domainname"] = parts[1]
	} else {
		h.Attributes["domainname"] = ""
	}
}

func (host Host) String() string {
	return fmt.Sprintf("Host{Name: %s, Keys: %d, Attributes: %s}", host.Name, len(host.publicKeys), host.Attributes)
}

// Adds a public key to a host. Used by the ssh know hosts provider, but can be
// used by any other code as well.
func (h *Host) AddPublicKey(k ssh.PublicKey) {
	h.publicKeys = append(h.publicKeys, k)
}

func (h *Host) PublicKeys() []ssh.PublicKey {
	return h.publicKeys
}

var _regexpType = reflect.TypeOf(regexp.MustCompile(""))
var _stringType = reflect.TypeOf("")

func (h *Host) Match(hostnameGlob string, attributes MatchAttributes) bool {

	if hostnameGlob != "" {
		ok, err := filepath.Match(hostnameGlob, h.Name)
		if !ok || err != nil {
			return false
		}
	}

	for _, attribute := range attributes {
		name := attribute.Name
		value, ok := h.GetAttribute(name)
		if !ok && !attribute.Negate {
			return false
		}
		if ok && !attribute.Match(value) {
			return false
		}
	}
	return true
}

func (h *Host) GetAttribute(key string) (interface{}, bool) {
	value, ok := h.Attributes[key]
	if ok {
		return value, ok
	}
	r := h.lastResult
	if r == nil {
		r = &Result{ExitStatus: -1}
	}
	switch key {
	case "name":
		return h.Name, true
	case "random":
		return h.csum, true
	case "address":
		return h.Address, true
	case "stdout":
		return string(r.Stdout), true
	case "stderr":
		return string(r.Stderr), true
	case "exitstatus":
		return r.ExitStatus, true
	case "err":
		return r.Err, true
	}
	return nil, false
}

func (h *Host) Amend(h2 *Host) {
	if h.Address == "" {
		h.Address = h2.Address
	}
	h.Attributes["herd_provider"] = append(h.Attributes["herd_provider"].([]string), h2.Attributes["herd_provider"].([]string)[0])
	for attr, value := range h2.Attributes {
		if attr == "herd_provider" {
			continue
		}
		h.Attributes[attr] = value
	}
	for _, k := range h2.publicKeys {
		h.AddPublicKey(k)
	}
}

func (h1 *Host) less(h2 *Host, attributes []string) bool {
	for _, attr := range attributes {
		v1, ok1 := h1.GetAttribute(attr)
		v2, ok2 := h2.GetAttribute(attr)
		// Sort nodes that are missing the attribute last
		if ok1 && !ok2 {
			return true
		}
		if !ok1 && ok2 {
			return false
		}
		if !ok1 && !ok2 {
			continue
		}
		// Compare the string values, this way we don't need to check a matrix of types
		s1, err1 := cast.ToStringE(v1)
		s2, err2 := cast.ToStringE(v2)
		if err1 != nil || err2 != nil {
			continue
		}
		// When equal, continue to the next field
		if s1 == s2 {
			continue
		}
		return s1 < s2
	}
	return h1.Name < h2.Name
}
