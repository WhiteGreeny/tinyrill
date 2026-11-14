package instance

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/mythologyli/zju-connect/client"
)

const resourceSummaryLimit = 20

type resourceRow struct {
	target, protocol string
	portMin, portMax int
}

// Resources describes the parsed authorization rules without probing targets or
// changing routing. Human-readable output stays separate from JSON status events.
func (f *CLIFrontend) Resources(generation uint64, ips []client.IPResource, domains client.DomainResources) {
	if f.ResourceMode == "none" || f.Diagnostic == nil {
		return
	}
	rows := make([]resourceRow, 0, len(ips)+len(domains))
	for _, ip := range ips {
		target := ip.IPMin.String()
		if !ip.IPMin.Equal(ip.IPMax) {
			target += " - " + ip.IPMax.String()
		}
		rows = append(rows, resourceRow{target, ip.Protocol, ip.PortMin, ip.PortMax})
	}
	for domain, resources := range domains {
		for _, r := range resources {
			rows = append(rows, resourceRow{domain, r.Protocol, r.PortMin, r.PortMax})
		}
	}
	slices.SortFunc(rows, func(a, b resourceRow) int {
		return cmp.Or(cmp.Compare(a.target, b.target), cmp.Compare(a.protocol, b.protocol), cmp.Compare(a.portMin, b.portMin), cmp.Compare(a.portMax, b.portMax))
	})
	rows = slices.Compact(rows) // Presentation only: distinct routing metadata is retained by the client.
	tcp := 0
	for _, row := range rows {
		if row.protocol == "tcp" || row.protocol == "all" {
			tcp++
		}
	}
	out := f.Diagnostic
	fmt.Fprintf(out, "\nCustomer %s connected (generation %d)\n", resourceText(f.Profile.ID), generation)
	if f.Profile.Listen.SOCKS5 != "" {
		fmt.Fprintf(out, "SOCKS5 listen: %s\n", resourceText(f.Profile.Listen.SOCKS5))
	}
	if f.Profile.Listen.HTTP != "" {
		fmt.Fprintf(out, "HTTP listen: %s\n", resourceText(f.Profile.Listen.HTTP))
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "No displayable authorization rules were returned. This does not confirm resource reachability.")
		return
	}
	fmt.Fprintf(out, "Authorized resources: %d distinct rules; %d permit TCP.\n", len(rows), tcp)
	fmt.Fprintln(out, "Gateway authorization only; reachability is not tested. HTTP/SOCKS5 support TCP only.")
	shown := len(rows)
	if f.ResourceMode != "full" {
		shown = min(shown, resourceSummaryLimit)
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "TARGET\tPROTOCOL\tPORTS\tTCP PROXY")
	for _, row := range rows[:shown] {
		ports := strconv.Itoa(row.portMin)
		if row.portMin != row.portMax {
			ports += "-" + strconv.Itoa(row.portMax)
		}
		access := "unsupported"
		if row.protocol == "tcp" || row.protocol == "all" {
			access = "permitted"
		} else if row.protocol == "icmp" {
			ports = "-"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", resourceText(row.target), resourceText(row.protocol), ports, access)
	}
	_ = table.Flush()
	if shown < len(rows) {
		fmt.Fprintf(out, "Showing %d of %d rules. Start with --resources full to show all, or --resources none to hide.\n", shown, len(rows))
	}
}

// Resource labels originate at the gateway. Keep each cell on one line and
// exclude terminal controls, bidi formatting and unbounded labels.
func resourceText(value string) string {
	var result strings.Builder
	count := 0
	for _, r := range value {
		if count == 256 {
			result.WriteString("...")
			break
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			r = '?'
		}
		result.WriteRune(r)
		count++
	}
	return result.String()
}
