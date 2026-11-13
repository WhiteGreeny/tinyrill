package authchallenge

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

var ErrRefreshCaptcha = errors.New("captcha refresh requested")

const captchaPageHTML = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>tinyrill 验证码</title><style>body{font:16px system-ui;max-width:800px;margin:40px auto;padding:16px;background:#f4f5f6;color:#18212b}main{background:white;padding:24px;border-radius:12px}#wrap{position:relative;display:inline-block;max-width:100%}img{display:block;max-width:100%;cursor:crosshair}.point{position:absolute;border-radius:50%;background:#006bc8;color:white;width:24px;height:24px;text-align:center;line-height:24px;transform:translate(-50%,-50%);pointer-events:none}button{font:inherit;padding:8px 16px;margin:12px 6px 0 0}#status{min-height:24px}</style>
<main><h1>按顺序点选验证码</h1><p>{{.Customer}} · 会话 {{.Generation}}</p><div id="wrap"><img id="picture" src="image" alt="验证码" draggable="false"></div><p id="status">请点击图片指定的位置</p><button id="undo">撤销</button><button id="clear">清除</button><button id="refresh">换一张</button><button id="submit">提交</button></main>
<script>
const picture=document.querySelector('#picture'),wrap=document.querySelector('#wrap'),status=document.querySelector('#status');let points=[],busy=false;
function render(){wrap.querySelectorAll('.point').forEach(e=>e.remove());points.forEach((p,i)=>{const m=document.createElement('span');m.className='point';m.textContent=i+1;m.style.left=(p[0]/picture.naturalWidth*100)+'%';m.style.top=(p[1]/picture.naturalHeight*100)+'%';wrap.appendChild(m)});status.textContent='已选择 '+points.length+' 个点'}
picture.onclick=e=>{if(busy||points.length>=16)return;const r=picture.getBoundingClientRect();points.push([Math.min(picture.naturalWidth-1,Math.max(0,Math.floor((e.clientX-r.left)*picture.naturalWidth/r.width))),Math.min(picture.naturalHeight-1,Math.max(0,Math.floor((e.clientY-r.top)*picture.naturalHeight/r.height)))]);render()};
document.querySelector('#undo').onclick=()=>{if(!busy){points.pop();render()}};document.querySelector('#clear').onclick=()=>{if(!busy){points=[];render()}};
async function send(refresh){if(busy||(!refresh&&!points.length))return;busy=true;try{const r=await fetch('submit',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:{{.ID}},coordinates:points,width:picture.naturalWidth,height:picture.naturalHeight,refresh})});if(!r.ok)throw Error();status.textContent=refresh?'已请求新验证码，请回终端打开新链接。':'已提交，请回终端查看认证结果。';picture.style.visibility='hidden';document.querySelectorAll('button').forEach(b=>b.disabled=true)}catch(e){status.textContent='提交未完成或挑战已失效，请回终端查看。';busy=false}}
document.querySelector('#submit').onclick=()=>send(false);document.querySelector('#refresh').onclick=()=>send(true);
</script></html>`

type callbackResult struct {
	response ClickCaptchaResponse
	err      error
}
type captchaCallback struct {
	challenge              Challenge
	basePath, origin, host string
	used                   atomic.Bool
	result                 chan callbackResult
}

func newCaptchaCallback(c Challenge, externalURL string) (*captchaCallback, error) {
	u, err := url.Parse(externalURL)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("callback URL must be an HTTP loopback origin")
	}
	host := net.ParseIP(u.Hostname())
	if host == nil || !host.IsLoopback() || u.Port() == "" {
		return nil, errors.New("callback URL must include a loopback IP and explicit port")
	}
	return &captchaCallback{challenge: c, basePath: "/c/" + rand.Text() + "/", origin: u.String(), host: u.Host, result: make(chan callbackResult, 1)}, nil
}

func (c *captchaCallback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	if r.Host != c.host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != c.origin) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "origin rejected", http.StatusForbidden)
		return
	}
	if time.Now().After(c.challenge.Meta.Expires) || c.used.Load() {
		http.Error(w, "challenge expired", http.StatusGone)
		return
	}
	switch r.URL.Path {
	case c.basePath:
		if r.Method != http.MethodGet {
			http.Error(w, "method rejected", 405)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = template.Must(template.New("captcha").Parse(captchaPageHTML)).Execute(w, c.challenge.Meta)
	case c.basePath + "image":
		if r.Method != http.MethodGet {
			http.Error(w, "method rejected", 405)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(c.challenge.Image))
		_, _ = w.Write(c.challenge.Image)
	case c.basePath + "submit":
		if r.Method != http.MethodPost {
			http.Error(w, "method rejected", 405)
			return
		}
		if r.Header.Get("Origin") != c.origin || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "origin or content type rejected", 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		var payload struct {
			ID          string  `json:"id"`
			Coordinates [][]int `json:"coordinates"`
			Width       int     `json:"width"`
			Height      int     `json:"height"`
			Refresh     bool    `json:"refresh"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&payload) != nil || payload.ID != c.challenge.Meta.ID {
			http.Error(w, "invalid response", 400)
			return
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			http.Error(w, "invalid response", 400)
			return
		}
		result := callbackResult{}
		if payload.Refresh {
			result.err = ErrRefreshCaptcha
		} else {
			points, err := pointsFromCoordinates(payload.Coordinates)
			points.Width, points.Height = payload.Width, payload.Height
			if err == nil {
				err = validateImagePoints(c.challenge.Image, points)
			}
			if err != nil {
				http.Error(w, "invalid coordinates", 400)
				return
			}
			result.response = points
		}
		if !c.used.CompareAndSwap(false, true) {
			http.Error(w, "already submitted", 409)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		c.result <- result
	default:
		http.NotFound(w, r)
	}
}

func (h *CLIHandler) clickInBrowser(ctx context.Context, challenge Challenge) (ClickCaptchaResponse, error) {
	listener, err := net.Listen("tcp", h.options.CallbackListen)
	if err != nil {
		return ClickCaptchaResponse{}, fmt.Errorf("reserve captcha callback: %w", err)
	}
	defer listener.Close()
	external := strings.TrimSuffix(h.options.CallbackURL, "/")
	if external == "" {
		external = "http://" + listener.Addr().String()
	}
	callback, err := newCaptchaCallback(challenge, external)
	if err != nil {
		return ClickCaptchaResponse{}, err
	}
	server := &http.Server{Handler: callback, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10}
	defer func() {
		closing, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(closing)
		_ = server.Close()
	}()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	fmt.Fprintf(h.options.Output, "Open captcha page (expires with this challenge): %s%s\n", external, callback.basePath)
	select {
	case result := <-callback.result:
		return result.response, result.err
	case <-ctx.Done():
		return ClickCaptchaResponse{}, ctx.Err()
	case err := <-serveErr:
		return ClickCaptchaResponse{}, err
	}
}
