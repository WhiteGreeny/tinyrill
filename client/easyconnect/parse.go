package easyconnect

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/internal/bounded"
	"github.com/mythologyli/zju-connect/log"
	"inet.af/netaddr"
)

func (c *Client) parseLineListFromConfig(config string) error {
	if err := bounded.XML(config); err != nil {
		return err
	}

	log.Println("Parsing line list from config...")

	doc := etree.NewDocument()

	err := doc.ReadFromString(config)
	if err != nil {
		return err
	}

	root := doc.SelectElement("Conf")
	if root == nil {
		return errors.New("no Conf element found")
	}
	element := root.SelectElement("Mline")
	if element == nil {
		return errors.New("no Mline element found")
	}

	if element.SelectAttrValue("enable", "") != "1" {
		return errors.New("server disable Mline")
	}

	if element.SelectAttr("list") == nil {
		return errors.New("no list attribute found")
	}

	lineListStr := element.SelectAttr("list").Value
	if lineListStr == "" {
		return errors.New("empty line list")
	}

	lineList := strings.Split(lineListStr, ";")
	if len(lineList) > 64 {
		return errors.New("too many gateway lines")
	}
	c.lineList = nil

	for _, line := range lineList {
		if line != "" {
			c.lineList = append(c.lineList, line)
		}
	}

	if len(c.lineList) == 0 {
		return errors.New("empty line list")
	}

	return nil
}

func (c *Client) parseResources(resources string) error {
	if err := bounded.XML(resources); err != nil {
		return err
	}

	log.Println("Parsing resources...")

	doc := etree.NewDocument()

	err := doc.ReadFromString(resources)
	if err != nil {
		return err
	}

	c.tcpTunnelOnly = false
	if root := doc.SelectElement("Resource"); root != nil {
		if other := root.SelectElement("Other"); other != nil {
			c.tcpTunnelOnly = other.SelectAttrValue("allocateVip", "") == "0"
		}
	}

	ipSetBuilder := netaddr.IPSetBuilder{}
	c.ipResources = make([]client.IPResource, 0)
	c.domainResources = make(client.DomainResources)
	c.dnsResource = make(map[string][]net.IP)

	root := doc.SelectElement("Resource")
	if root == nil {
		return errors.New("no Resource element found")
	}
	element := root.SelectElement("Rcs")
	c.resourceDiagnostics.EasyConnect(element)
	if element == nil {
		return errors.New("no Rcs element found")
	}

	count := 0
	for _, rc := range element.SelectElements("Rc") {
		if rc.SelectAttrValue("type", "") == "1" || rc.SelectAttrValue("type", "") == "2" {
			// etree decodes XML entities. Keep the gateway's labels as display
			// metadata; it must not affect target, port or protocol matching.
			name := rc.SelectAttrValue("name", "")
			note := rc.SelectAttrValue("note", "")
			var protocol string
			protoStr := rc.SelectAttrValue("proto", "")
			if protoStr == "-1" {
				protocol = "all"
			} else if protoStr == "0" {
				protocol = "tcp"
			} else if protoStr == "1" {
				protocol = "udp"
			} else if protoStr == "2" {
				protocol = "icmp"
			} else {
				log.DebugPrintf("Unknown protocol: %s", protoStr)
				continue
			}

			hostListStr := rc.SelectAttr("host")
			if hostListStr == nil {
				continue
			}
			hostList := strings.Split(hostListStr.Value, ";")

			portRangeListStr := rc.SelectAttr("port")
			if portRangeListStr == nil {
				continue
			}
			portRangeList := strings.Split(portRangeListStr.Value, ";")

			if len(hostList) != len(portRangeList) {
				log.DebugPrintln("Host and port list length mismatch, skip")
				continue
			}

			for i, host := range hostList {
				count++
				if count > 16384 {
					return errors.New("too many gateway resources")
				}
				portRangeStr := portRangeList[i]
				portRange := strings.Split(portRangeStr, "~")
				if len(portRange) != 2 {
					log.DebugPrintf("Invalid port range: %s", portRangeStr)
					continue
				}
				portMin, err := strconv.Atoi(portRange[0])
				if err != nil {
					log.DebugPrintf("Invalid port range: %s", portRangeStr)
					continue
				}
				portMax, err := strconv.Atoi(portRange[1])
				if err != nil || portMin < 0 || portMax > 65535 || portMin > portMax {
					log.DebugPrintf("Invalid port range: %s", portRangeStr)
					continue
				}

				isDomain := false
				var ipMin net.IP
				var ipMax net.IP
				var hostPort string
				if strings.Contains(host, "~") {
					ipList := strings.Split(host, "~")
					if len(ipList) != 2 {
						log.DebugPrintf("Invalid IP range: %s", host)
						continue
					}
					ipMin = net.ParseIP(ipList[0])
					if ipMin == nil || ipMin.To4() == nil {
						log.DebugPrintf("Invalid IP range: %s", host)
						continue
					}
					ipMax = net.ParseIP(ipList[1])

					if ipMax == nil || ipMax.To4() == nil {
						log.DebugPrintf("Invalid IP range: %s", host)
						continue
					}

					ipSetBuilder.AddRange(netaddr.IPRangeFrom(netaddr.MustParseIP(ipList[0]), netaddr.MustParseIP(ipList[1])))
					log.DebugPrintf("Add IP range: %s ~ %s, Port range: %d ~ %d, [%s]", ipList[0], ipList[1], portMin, portMax, protocol)
				} else {
					if strings.Contains(host, "//") {
						host = strings.Split(host, "//")[1]
					}
					host = strings.Split(host, "/")[0]
					host = strings.ReplaceAll(host, "*", "")
					if strings.Contains(host, ":") {
						host, hostPort, err = net.SplitHostPort(host)
						if err != nil {
							continue
						}
					}
					if host == "" || len(host) > 253 {
						continue
					}
					ipMin = net.ParseIP(host)
					if ipMin == nil {
						isDomain = true

						if !c.useDomainResource {
							continue
						}

						log.DebugPrintf("Add domain: %s, Port range: %d ~ %d, [%s]", host, portMin, portMax, protocol)
					} else {
						if ipMin.To4() == nil {
							continue
						}
						ipMax = ipMin
						if hostPortInt, err := strconv.Atoi(hostPort); hostPort != "" && err == nil {
							portMin = hostPortInt
							portMax = hostPortInt
						}

						ipSetBuilder.Add(netaddr.MustParseIP(host))
						log.DebugPrintf("Add IP: %s, Port range: %d ~ %d, [%s]", host, portMin, portMax, protocol)
					}
				}

				if isDomain {
					c.domainResources[host] = append(c.domainResources[host], client.DomainResource{
						Name:     name,
						Note:     note,
						PortMin:  portMin,
						PortMax:  portMax,
						Protocol: protocol,
					})
				} else {
					c.ipResources = append(c.ipResources, client.IPResource{
						Name:     name,
						Note:     note,
						IPMin:    ipMin,
						IPMax:    ipMax,
						PortMin:  portMin,
						PortMax:  portMax,
						Protocol: protocol,
					})
				}
			}
		}
	}

	c.ipSet, err = ipSetBuilder.IPSet()
	if err != nil {
		return err
	}
	c.dnsServer = ""
	c.dnsServers = nil
	element = root.SelectElement("Dns")
	if element == nil {
		return nil
	}

	for _, dnsStr := range strings.Split(element.SelectAttrValue("data", ""), ";") {
		if dnsStr == "" {
			continue
		}

		dnsParts := strings.Split(dnsStr, ":")
		if len(dnsParts) != 3 {
			continue
		}

		ip, err := netaddr.ParseIP(dnsParts[2])
		if err != nil {
			continue
		}

		c.dnsResource[dnsParts[1]] = append(c.dnsResource[dnsParts[1]], ip.IPAddr().IP)
		log.DebugPrintf("Add DNS rule: %s -> %s", dnsParts[1], dnsParts[2])
	}

	for _, server := range strings.Split(element.SelectAttrValue("dnsserver", ""), ";") {
		if ip := net.ParseIP(server); ip != nil && ip.To4() != nil && !ip.IsUnspecified() {
			c.dnsServers = append(c.dnsServers, server)
		}
	}
	if len(c.dnsServers) > 0 {
		c.dnsServer = c.dnsServers[0]
	}

	if c.dnsServer == "0.0.0.0" {
		c.dnsServer = ""
	}

	return nil
}
