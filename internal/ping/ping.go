package ping

import (
	"bytes"
	"fmt"
	"html/template"
	"time"
)

// Protocol ...
type Protocol int

func (protocol Protocol) String() string {
	switch protocol {
	case TCP:
		return "tcp"
	}
	return "unknown"
}

// TCP is the protocol used for VPN gateway latency probes.
const TCP Protocol = 0

// Target is a ping
type Target struct {
	Protocol Protocol
	Host     string
	Port     int

	Counter  int
	Interval time.Duration
	Timeout  time.Duration
}

func (target Target) String() string {
	return fmt.Sprintf("%s://%s:%d", target.Protocol, target.Host, target.Port)
}

// Pinger is a ping interface
type Pinger interface {
	Start() <-chan struct{}
	Stop()
	Result() *Result
	SetTarget(target *Target)
}

// Result ...
type Result struct {
	Counter        int
	SuccessCounter int
	Target         *Target

	MinDuration   time.Duration
	MaxDuration   time.Duration
	TotalDuration time.Duration
}

// Avg return the average time of ping
func (result Result) Avg() time.Duration {
	if result.SuccessCounter == 0 {
		return 0
	}
	return result.TotalDuration / time.Duration(result.SuccessCounter)
}

// Failed return failed counter
func (result Result) Failed() int {
	return result.Counter - result.SuccessCounter
}

func (result Result) String() string {
	const resultTpl = `
Ping statistics {{.Target}}
	{{.Counter}} probes sent.
	{{.SuccessCounter}} successful, {{.Failed}} failed.
Approximate trip times:
	Minimum = {{.MinDuration}}, Maximum = {{.MaxDuration}}, Average = {{.Avg}}`
	t := template.Must(template.New("result").Parse(resultTpl))
	res := bytes.NewBufferString("")
	t.Execute(res, result)
	return res.String()
}
