package herd

import (
	"encoding/json"
	"testing"
)

func TestHostDeserialization(t *testing.T) {
	data := map[string]interface{}{
		"Name": "test-host.herd.ci",
		"Attributes": map[string]interface{}{
			"color":  "puce",
			"number": 32,
			"float":  1.1,
		},
	}
	bdata, _ := json.Marshal(data)
	var host Host
	err := json.Unmarshal(bdata, &host)
	if err != nil {
		t.Errorf("Unable to deserialize host data: %s", err)
	}
	if host.Attributes == nil {
		t.Errorf("Deserialized host does not get attributes")
	}
	if domain, ok := host.Attributes["domainname"]; !ok || domain != "herd.ci" {
		t.Errorf("Deserialized host is missing attributes")
	}
	if color, ok := host.Attributes["color"]; !ok || color != "puce" {
		t.Errorf("string attribute did not survive the json trip: %v", host.Attributes)
	}
	if number, ok := host.Attributes["number"]; !ok || number != int64(32) {
		t.Errorf("integer attribute did not survive the json trip: %v", host.Attributes)
	}
	if flt, ok := host.Attributes["float"]; !ok || flt != 1.1 {
		t.Errorf("float attribute did not survive the json trip: %v", host.Attributes)
	}
}

func TestHostSorting(t *testing.T) {
	h1 := NewHost("host-a.example.com", "", HostAttributes{"site": "site1", "role": "db"})
	h2 := NewHost("host-b.example.com", "", HostAttributes{"site": "site2", "role": "db"})
	h3 := NewHost("host-c.example.com", "", HostAttributes{"site": "site1", "role": "app"})
	h4 := NewHost("host-d.example.com", "", HostAttributes{"site": "site2", "role": "app"})
	hosts := Hosts{h1, h2, h3, h4}

	if !h1.less(h2, []string{}) {
		t.Errorf("Sorting hosts with no fields is failing")
	}
	if h2.less(h1, []string{}) {
		t.Errorf("Sorting hosts with no fields is failing")
	}
	if !h1.less(h2, []string{"name"}) {
		t.Errorf("Sorting hosts by name is failing")
	}
	if h2.less(h1, []string{"name"}) {
		t.Errorf("Sorting hosts by name fields is failing")
	}

	hosts.Sort([]string{"site", "name"})
	if !eq(hosts, Hosts{h1, h3, h2, h4}) {
		t.Errorf("Sorting by site+name is failing, got %v", hosts)
	}

	hosts.Sort([]string{"site", "role"})
	if !eq(hosts, Hosts{h3, h1, h4, h2}) {
		t.Errorf("Sorting by site+role is failing, got %v", hosts)
	}
}

func eq(a, b Hosts) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
