package easyconnect

import "testing"

func TestMalformedResourcesNeverPanic(t *testing.T) {
	for _, s := range []string{"", `<Wrong/>`, `<Resource/>`, `<Resource><Rcs><Rc/></Rcs></Resource>`, `<Resource><Rcs><Rc type="1"/></Rcs></Resource>`} {
		c := &Client{}
		_ = c.parseResources(s)
		_ = c.parseLineListFromConfig(s)
	}
	c := &Client{}
	if err := c.parseResources(`<Resource><Rcs><Rc type="1" proto="0" host="10.1.1.1" port="443~443"/></Rcs></Resource>`); err != nil {
		t.Fatal(err)
	}
	if c.ipSet == nil || len(c.ipResources) != 1 || len(c.dnsServers) != 0 {
		t.Fatal("IP-only gateway resources must work without DNS")
	}
	for _, s := range []string{"", `<TwfID/>`, `<TwfID></TwfID>`} {
		if _, err := requiredLoginField([]byte(s), "TwfID"); err == nil {
			t.Fatal("missing field accepted")
		}
	}
}
func FuzzResourceParser(f *testing.F) {
	f.Add(`<Resource><Rcs/><Dns/></Resource>`)
	f.Add(`<Conf><Mline enable="1" list="vpn.example:443"/></Conf>`)
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<20 {
			return
		}
		c := &Client{useDomainResource: true}
		_ = c.parseResources(s)
		_ = c.parseLineListFromConfig(s)
	})
}
